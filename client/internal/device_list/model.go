package device_list

// DeviceInfo 设备基础信息，Android 和 iOS 共用这一套结构，前端只需要解析一个 devices 数组。
type DeviceInfo struct {
	Serial         string `json:"serial"`                    // Android 是 adb 序列号；iOS 优先返回硬件序列号，没有时返回 UDID
	DeviceID       string `json:"device_id"`                 // 设备唯一标识：Android serial / iOS UDID，后续接口用它定位设备
	DeviceName     string `json:"device_name,omitempty"`     // iOS 设备名，例如 iPhone；Android 通常为空
	Platform       string `json:"platform"`                  // android / ios
	Version        string `json:"version"`                   // 系统版本，例如 15、26.2.1
	Brand          string `json:"brand"`                     // Android 品牌；iOS 固定 Apple
	Model          string `json:"model"`                     // Android 型号；iOS 优先 market_name，其次 ProductType
	ProductType    string `json:"product_type,omitempty"`    // iOS 机型标识，例如 iPhone12,1
	ConnectionType string `json:"connection_type,omitempty"` // iOS 连接方式，例如 usb / network
	Status         string `json:"status"`                    // Online / Offline / Unauthorized
	Error          string `json:"error,omitempty"`           // 获取设备信息失败原因；正常时不返回
}

type IOSDeviceInfo struct {
	DeviceName     string `json:"device_name"`
	DeviceID       string `json:"device_id"`
	ProductType    string `json:"product_type"`
	ProductVersion string `json:"product_version"`
}

//{ "BuildVersion": "23G5028e",
//	"ConnectionType": "USB",
//	"DeviceClass": "iPhone",
//	"DeviceName": "iPhone15test",
//	"Identifier": "00008120-00024C6C0A3B601E",
//	"ProductType": "iPhone15,4",
//	"ProductVersion": "26.6",
//	"UniqueDeviceID": "00008120-00024C6C0A3B601E" }
