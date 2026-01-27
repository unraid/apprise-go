package notify

import (
	"fmt"
	"strconv"
	"strings"
)

const appriseImageURLMask = "https://raw.githubusercontent.com/unraid/apprise-go/main/assets/themes/default/apprise-%s-%s.png"
const appriseDefaultColor = "#888888"
const appriseAppURL = "https://github.com/unraid/apprise-go"

func appriseImageURL(notifyType NotifyType, size string) string {
	if size == "" {
		size = "256x256"
	}
	return fmt.Sprintf(appriseImageURLMask, string(notifyType), size)
}

func appriseColor(notifyType NotifyType) string {
	switch notifyType {
	case NotifyInfo:
		return "#3AA3E3"
	case NotifySuccess:
		return "#3AA337"
	case NotifyFailure:
		return "#A32037"
	case NotifyWarning:
		return "#CACF29"
	default:
		return appriseDefaultColor
	}
}

func appriseColorInt(notifyType NotifyType) int {
	color := strings.TrimPrefix(appriseColor(notifyType), "#")
	value, err := strconv.ParseInt(color, 16, 64)
	if err != nil {
		return 0
	}
	return int(value)
}
