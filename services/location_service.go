package services

import (
	"fmt"
	"sync"

	"github.com/danielpaulus/go-ios/ios"

	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/danielpaulus/go-ios/ios/simlocation"
)

// LocationService 位置模拟服务
type LocationService struct {
	mu              sync.Mutex
	locationServers map[string]*instruments.LocationSimulationService
}

// getTunneledDevice 获取通过隧道连接的设备（iOS 17+）
func getTunneledDevice(udid string) (ios.DeviceEntry, error) {
	device, err := ios.GetDevice(udid)
	if err != nil {
		return ios.DeviceEntry{}, fmt.Errorf("获取设备失败: %w", err)
	}

	info, err := GetTunnelForDevice(udid)
	if err != nil {
		return ios.DeviceEntry{}, fmt.Errorf("获取隧道信息失败: %w", err)
	}

	device.UserspaceTUNPort = info.UserspaceTUNPort
	device.UserspaceTUNHost = "localhost"
	device.UserspaceTUN = info.UserspaceTUN

	rsdService, err := ios.NewWithAddrPortDevice(info.Address, info.RsdPort, device)
	if err != nil {
		return ios.DeviceEntry{}, fmt.Errorf("连接 RSD 失败: %w", err)
	}
	defer rsdService.Close()

	rsdProvider, err := rsdService.Handshake()
	if err != nil {
		return ios.DeviceEntry{}, fmt.Errorf("RSD 握手失败: %w", err)
	}

	tunDevice, err := ios.GetDeviceWithAddress(udid, info.Address, rsdProvider)
	if err != nil {
		return ios.DeviceEntry{}, fmt.Errorf("获取隧道设备失败: %w", err)
	}
	tunDevice.UserspaceTUN = device.UserspaceTUN
	tunDevice.UserspaceTUNHost = device.UserspaceTUNHost
	tunDevice.UserspaceTUNPort = device.UserspaceTUNPort

	return tunDevice, nil
}

func (l *LocationService) SetLocation(udid string, lat, lon float64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.locationServers == nil {
		l.locationServers = make(map[string]*instruments.LocationSimulationService)
	}

	// iOS 17+ 使用隧道 + instruments 服务
	if IsVersionAbove17(udid) {
		server, exists := l.locationServers[udid]
		if !exists {
			tunDevice, err := getTunneledDevice(udid)
			if err != nil {
				return fmt.Errorf("获取隧道设备失败: %w", err)
			}
			server, err = instruments.NewLocationSimulationService(tunDevice)
			if err != nil {
				return fmt.Errorf("创建位置模拟服务失败: %w", err)
			}
			l.locationServers[udid] = server
		}

		err := server.StartSimulateLocation(lat, lon)
		if err != nil {
			return fmt.Errorf("启动位置模拟失败: %w", err)
		}
		return nil
	}

	// iOS <17 使用旧版 simlocation 服务
	device, err := ios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("获取设备失败: %w", err)
	}
	err = simlocation.SetLocation(device, fmt.Sprintf("%f", lat), fmt.Sprintf("%f", lon))
	if err != nil {
		Log.Error("LocationService", fmt.Sprintf("设置位置失败 for %s: %v", udid, err))
		return fmt.Errorf("设置位置失败: %w", err)
	}

	return nil
}

// ResetLocation 重置设备位置
func (l *LocationService) ResetLocation(udid string) error {
	Log.Info("LocationService", fmt.Sprintf("重置设备 %s 位置...", udid))
	l.mu.Lock()
	defer l.mu.Unlock()

	if server, exists := l.locationServers[udid]; exists {
		err := server.StopSimulateLocation()
		if err != nil {
			Log.Error("LocationService", fmt.Sprintf("停止位置模拟失败 for %s: %v", udid, err))
		}
		delete(l.locationServers, udid)
	}

	// iOS 17+ 只需要停止 instruments 服务即可
	if IsVersionAbove17(udid) {
		Log.Info("LocationService", fmt.Sprintf("设备 %s 位置已重置", udid))
		return nil
	}

	// iOS <17 使用旧版 simlocation 重置
	device, err := ios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("获取设备失败: %w", err)
	}

	err = simlocation.ResetLocation(device)
	if err != nil {
		Log.Error("LocationService", fmt.Sprintf("重置位置失败 for %s: %v", udid, err))
		return fmt.Errorf("重置位置失败: %w", err)
	}

	Log.Info("LocationService", fmt.Sprintf("设备 %s 位置已重置", udid))
	return nil
}
