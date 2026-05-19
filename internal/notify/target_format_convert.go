package notify

import "strings"

func ConvertMessageFormatForTarget(parsed *ParsedURL, content, inputFormat string) (string, error) {
	if parsed != nil && strings.EqualFold(parsed.Scheme, "tgram") {
		return convertTelegramMessageFormat(content, inputFormat, parsed.Query["format"], parsed.Query["mdv"])
	}

	outputFormat := ""
	if parsed != nil {
		outputFormat = parsed.Query["format"]
	}
	return ConvertMessageFormat(content, inputFormat, outputFormat)
}
