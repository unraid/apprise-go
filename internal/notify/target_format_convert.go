package notify

import "strings"

func ConvertMessageFormatForTarget(parsed *ParsedURL, content, inputFormat string) (string, error) {
	if parsed != nil && strings.EqualFold(parsed.Scheme, "tgram") {
		return convertTelegramMessageFormat(content, inputFormat, parsed.Query["format"], parsed.Query["mdv"])
	}

	// The target format is the service's own unless the URL overrides it.
	// Reading only ?format= meant a markdown-native service handed HTML got
	// the HTML through untouched, where upstream converts it — which is what
	// the four "splitting override" plugins actually differ on.
	outputFormat := ""
	if parsed != nil {
		outputFormat = strings.TrimSpace(parsed.Query["format"])
		if outputFormat == "" {
			outputFormat = OverflowLimitsFor(parsed.Scheme).Format
		}
	}

	return ConvertMessageFormat(content, inputFormat, outputFormat)
}
