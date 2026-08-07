package get_device_apps

import (
	"client/common"
	"client/internal/device_list"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"
)

func GetDeviceApps(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.PathValue("deviceId"))
	if deviceID == "" {
		common.Fail(w, 10004, "设备ID不能为空")
		return
	}

	uuid, err := device_list.GetAndroidUUID()
	serials := []string{}
	if err == nil {
		serials = device_list.ParseDeviceList(uuid)
	}

	if contains(serials, deviceID) {
		// Android 原逻辑不动：只读取当前用户下安装的第三方应用包名。
		// 部分 ROM 存在隐私空间/多用户，例如 user 666。
		// 不指定 user 时，pm 可能打印 “Shell does not have permission to access user 666”。
		// 这里优先使用当前用户，拿不到时回退到主用户 0。
		currentUser, _ := common.AdbShell(deviceID, "am get-current-user")
		currentUser = strings.TrimSpace(currentUser)
		if currentUser == "" || strings.HasPrefix(currentUser, "Error:") {
			currentUser = "0"
		}

		apps, err := common.AdbShell(
			deviceID,
			"pm list packages -3 --user "+currentUser,
		)
		if err != nil {
			common.Fail(w, 10003, "获取设备应用列表失败")
			return
		}

		result := make([]AppResponse, 0)

		for _, line := range strings.Split(apps, "\n") {
			packageName := strings.TrimSpace(line)
			// 跳过空行和异常文本
			if packageName == "" || strings.HasPrefix(packageName, "Error:") {
				continue
			}
			packageName = strings.TrimPrefix(packageName, "package:")
			if packageName == "" {
				continue
			}

			result = append(result, AppResponse{
				PackageName: packageName,
			})

		}

		// 正常返回
		common.Success(w, GetAppsResponse{
			Total: len(result),
			Items: result,
		})
		return
	}

	// 没命中 Android 设备时，按 iOS UDID 查询应用。
	// pymobiledevice3 返回结构是：bundleID -> appInfo；bundleID 就是 iOS 采集时要传的包名。
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	pythonCommand, pythonErr := common.PythonCommand()
	if pythonErr != nil {
		common.Fail(w, 10003, "未检测到 Python 或 pymobiledevice3")
		return
	}

	out, err := exec.CommandContext(
		ctx,
		pythonCommand,
		"-m",
		"pymobiledevice3",
		"apps",
		"list",
		"--udid",
		deviceID,
	).CombinedOutput()
	if err != nil {
		common.Fail(w, 10003, "获取 iOS 应用列表失败，请检查 pymobiledevice3 是否可用")
		return
	}

	var apps map[string]map[string]interface{}
	if err := json.Unmarshal(out, &apps); err != nil {
		common.Fail(w, 10006, "指定设备不在线，或 iOS 设备未信任当前电脑")
		return
	}

	result := make([]AppResponse, 0, len(apps))
	for bundleID, appInfo := range apps {
		bundleID = strings.TrimSpace(bundleID)
		if bundleID == "" {
			continue
		}

		// iOS 的中文名一般在 CFBundleDisplayName；没有时再用 CFBundleName。
		appName, _ := appInfo["CFBundleDisplayName"].(string)
		if strings.TrimSpace(appName) == "" {
			appName, _ = appInfo["CFBundleName"].(string)
		}

		executable, _ := appInfo["CFBundleExecutable"].(string)
		result = append(result, AppResponse{
			AppName:     strings.TrimSpace(appName),
			PackageName: bundleID,
			Executable:  strings.TrimSpace(executable),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		left := result[i].AppName
		if left == "" {
			left = result[i].PackageName
		}
		right := result[j].AppName
		if right == "" {
			right = result[j].PackageName
		}
		return left < right
	})

	common.Success(w, GetAppsResponse{
		Total: len(result),
		Items: result,
	})

}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}

	return false
}
