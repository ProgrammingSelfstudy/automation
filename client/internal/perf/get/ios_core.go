package get

import "strings"

const defaultIOSCPUCoreCount = 6

// IOSCPUCoreMap 是 iOS 机型 CPU 核心数映射表。
// key 支持两类：
//  1. ProductType：例如 iPhone12,1
//  2. 展示名归一化：例如 "iPhone 11" 会归一化成 "iphone11"
//
// 后面你要补新机型，只改这个文件即可。
var IOSCPUCoreMap = map[string]int{
	// iPhone 17 / Air / 17e
	"iphone18,1":     6,
	"iphone18,2":     6,
	"iphone18,3":     6,
	"iphone18,4":     6,
	"iphone18,5":     6,
	"iphone17pro":    6,
	"iphone17promax": 6,
	"iphone17":       6,
	"iphoneair":      6,
	"iphone17e":      6,

	// iPhone 16 / 16e
	"iphone17,1":     6,
	"iphone17,2":     6,
	"iphone17,3":     6,
	"iphone17,4":     6,
	"iphone17,5":     6,
	"iphone16pro":    6,
	"iphone16promax": 6,
	"iphone16":       6,
	"iphone16plus":   6,
	"iphone16e":      6,

	// iPhone 15
	"iphone16,1":     6,
	"iphone16,2":     6,
	"iphone15,4":     6,
	"iphone15,5":     6,
	"iphone15pro":    6,
	"iphone15promax": 6,
	"iphone15":       6,
	"iphone15plus":   6,

	// iPhone 14
	"iphone15,2":     6,
	"iphone15,3":     6,
	"iphone14,7":     6,
	"iphone14,8":     6,
	"iphone14pro":    6,
	"iphone14promax": 6,
	"iphone14":       6,
	"iphone14plus":   6,

	// iPhone SE 3 / iPhone 13
	"iphone14,6":     6,
	"iphone14,2":     6,
	"iphone14,3":     6,
	"iphone14,4":     6,
	"iphone14,5":     6,
	"iphonese3":      6,
	"iphone13pro":    6,
	"iphone13promax": 6,
	"iphone13mini":   6,
	"iphone13":       6,

	// iPhone 12
	"iphone13,1":     6,
	"iphone13,2":     6,
	"iphone13,3":     6,
	"iphone13,4":     6,
	"iphone12mini":   6,
	"iphone12":       6,
	"iphone12pro":    6,
	"iphone12promax": 6,

	// iPhone SE 2 / iPhone 11
	"iphone12,8":     6,
	"iphone12,1":     6,
	"iphone12,3":     6,
	"iphone12,5":     6,
	"iphonese2":      6,
	"iphone11":       6,
	"iphone11pro":    6,
	"iphone11promax": 6,

	// iPhone XS / XR / X / 8
	"iphone11,2":  6,
	"iphone11,4":  6,
	"iphone11,6":  6,
	"iphone11,8":  6,
	"iphone10,3":  6,
	"iphone10,6":  6,
	"iphone10,1":  6,
	"iphone10,4":  6,
	"iphone10,2":  6,
	"iphone10,5":  6,
	"iphonexs":    6,
	"iphonexsmax": 6,
	"iphonexr":    6,
	"iphonex":     6,
	"iphone8":     6,
	"iphone8plus": 6,

	// iPhone 7 / SE 1 / 6s / 6 / 5s
	"iphone9,1":    4,
	"iphone9,3":    4,
	"iphone9,2":    4,
	"iphone9,4":    4,
	"iphone7":      4,
	"iphone7plus":  4,
	"iphone8,4":    2,
	"iphonese":     2,
	"iphone8,1":    2,
	"iphone8,2":    2,
	"iphone6s":     2,
	"iphone6splus": 2,
	"iphone7,2":    2,
	"iphone7,1":    2,
	"iphone6":      2,
	"iphone6plus":  2,
	"iphone6,1":    2,
	"iphone6,2":    2,
	"iphone5s":     2,
}

func IOSCPUCoreCount(deviceModel string) int {
	key := normalizeIOSModelKey(deviceModel)
	if coreCount, exists := IOSCPUCoreMap[key]; exists && coreCount > 0 {
		return coreCount
	}

	return defaultIOSCPUCoreCount
}

func normalizeIOSModelKey(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", "（", "", "）", "", "(", "", ")", "")
	return replacer.Replace(key)
}
