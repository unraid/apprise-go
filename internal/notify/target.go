package notify

import (
	"errors"
	"fmt"
	"strings"
)

type Sender interface {
	Send(body, title string, notifyType NotifyType) error
}

type buildTargetFunc func(*ParsedURL) (Sender, error)

type UnsupportedSchemaError struct {
	Schema string
}

func (e *UnsupportedSchemaError) Error() string {
	return "unsupported url schema: " + e.Schema
}

func IsUnsupportedSchema(err error) bool {
	var schemaErr *UnsupportedSchemaError
	return errors.As(err, &schemaErr)
}

func NewTarget(parsed *ParsedURL) (Sender, error) {
	if parsed == nil {
		return nil, fmt.Errorf("nil parsed url")
	}
	builder, ok := targetBuilders[strings.ToLower(parsed.Scheme)]
	if !ok {
		return nil, &UnsupportedSchemaError{Schema: parsed.Scheme}
	}
	return builder(parsed)
}

func SendTargetURL(rawURL, body, title, inputFormat string, notifyType NotifyType) error {
	parsed, err := ParseURL(rawURL)
	if err != nil {
		return err
	}

	sendBody, err := ConvertMessageFormatForTarget(parsed, body, inputFormat)
	if err != nil {
		return err
	}

	target, err := NewTarget(parsed)
	if err != nil {
		return err
	}
	return target.Send(sendBody, title, notifyType)
}

func TargetSchemaName(schema string) string {
	switch strings.ToLower(schema) {
	case "apprises":
		return "apprise"
	case "barks":
		return "bark"
	case "wxteams":
		return "wxteams"
	case "victorops":
		return "splunk"
	case "workflows":
		return "workflow"
	case "elks":
		return "46elks"
	case "nexmo":
		return "vonage"
	case "smseagles":
		return "smseagle"
	case "smsmgr":
		return "smsmanager"
	case "signals":
		return "signal"
	case "rockets":
		return "rocket"
	case "mmosts":
		return "mmost"
	case "nctalks":
		return "nctalk"
	case "psafers":
		return "psafer"
	case "lametrics":
		return "lametric"
	case "nclouds":
		return "ncloud"
	case "mastodons", "toot", "toots":
		return "mastodon"
	case "misskeys":
		return "misskey"
	case "bsky":
		return "bluesky"
	case "tweet", "x":
		return "twitter"
	case "matrixs":
		return "matrix"
	case "jsons":
		return "json"
	case "forms":
		return "form"
	case "gotifys":
		return "gotify"
	case "pushdeers":
		return "pushdeer"
	case "pjet", "pjets":
		return "pushjet"
	case "pbul":
		return "pushbullet"
	case "pover":
		return "pushover"
	case "noticas":
		return "notica"
	case "mailtos":
		return "mailto"
	case "mqtts":
		return "mqtt"
	case "smpps":
		return "smpp"
	case "azure", "o365":
		return "office365"
	case "enigma2s":
		return "enigma2"
	case "embys":
		return "emby"
	case "xbmcs", "kodi", "kodis":
		return "xbmc"
	case "hassios":
		return "hassio"
	case "napi":
		return "notificationapi"
	case "parseps":
		return "parsep"
	case "synologys":
		return "synology"
	default:
		return strings.ToLower(schema)
	}
}

var targetBuilders = map[string]buildTargetFunc{
	"46elks": func(parsed *ParsedURL) (Sender, error) {
		return NewFortySixElksTarget(parsed)
	},
	"apprise": func(parsed *ParsedURL) (Sender, error) {
		return NewAppriseTarget(parsed)
	},
	"apprises": func(parsed *ParsedURL) (Sender, error) {
		return NewAppriseTarget(parsed)
	},
	"aprs": func(parsed *ParsedURL) (Sender, error) {
		return NewAprsTarget(parsed)
	},
	"atalk": func(parsed *ParsedURL) (Sender, error) {
		return NewAfricasTalkingTarget(parsed)
	},
	"azure": func(parsed *ParsedURL) (Sender, error) {
		return NewOffice365Target(parsed)
	},
	"bark": func(parsed *ParsedURL) (Sender, error) {
		return NewBarkTarget(parsed)
	},
	"barks": func(parsed *ParsedURL) (Sender, error) {
		return NewBarkTarget(parsed)
	},
	"bluesky": func(parsed *ParsedURL) (Sender, error) {
		return NewBlueskyTarget(parsed)
	},
	"brevo": func(parsed *ParsedURL) (Sender, error) {
		return NewBrevoTarget(parsed)
	},
	"bsky": func(parsed *ParsedURL) (Sender, error) {
		return NewBlueskyTarget(parsed)
	},
	"bulksms": func(parsed *ParsedURL) (Sender, error) {
		return NewBulkSMSTarget(parsed)
	},
	"bulkvs": func(parsed *ParsedURL) (Sender, error) {
		return NewBulkVSTarget(parsed)
	},
	"burstsms": func(parsed *ParsedURL) (Sender, error) {
		return NewBurstSMSTarget(parsed)
	},
	"chanify": func(parsed *ParsedURL) (Sender, error) {
		return NewChanifyTarget(parsed)
	},
	"clickatell": func(parsed *ParsedURL) (Sender, error) {
		return NewClickatellTarget(parsed)
	},
	"clicksend": func(parsed *ParsedURL) (Sender, error) {
		return NewClickSendTarget(parsed)
	},
	"d7sms": func(parsed *ParsedURL) (Sender, error) {
		return NewD7NetworksTarget(parsed)
	},
	"dapnet": func(parsed *ParsedURL) (Sender, error) {
		return NewDapnetTarget(parsed)
	},
	"dbus": func(parsed *ParsedURL) (Sender, error) {
		return NewLocalNotifyTarget(parsed)
	},
	"dingtalk": func(parsed *ParsedURL) (Sender, error) {
		return NewDingTalkTarget(parsed)
	},
	"discord": func(parsed *ParsedURL) (Sender, error) {
		return NewDiscordTarget(parsed)
	},
	"dot": func(parsed *ParsedURL) (Sender, error) {
		return NewDotTarget(parsed)
	},
	"elks": func(parsed *ParsedURL) (Sender, error) {
		return NewFortySixElksTarget(parsed)
	},
	"emby": func(parsed *ParsedURL) (Sender, error) {
		return NewEmbyTarget(parsed)
	},
	"embys": func(parsed *ParsedURL) (Sender, error) {
		return NewEmbyTarget(parsed)
	},
	"enigma2": func(parsed *ParsedURL) (Sender, error) {
		return NewEnigma2Target(parsed)
	},
	"enigma2s": func(parsed *ParsedURL) (Sender, error) {
		return NewEnigma2Target(parsed)
	},
	"fcm": func(parsed *ParsedURL) (Sender, error) {
		return NewFCMTarget(parsed)
	},
	"feishu": func(parsed *ParsedURL) (Sender, error) {
		return NewFeishuTarget(parsed)
	},
	"flock": func(parsed *ParsedURL) (Sender, error) {
		return NewFlockTarget(parsed)
	},
	"form": func(parsed *ParsedURL) (Sender, error) {
		return NewFormTarget(parsed)
	},
	"forms": func(parsed *ParsedURL) (Sender, error) {
		return NewFormTarget(parsed)
	},
	"freemobile": func(parsed *ParsedURL) (Sender, error) {
		return NewFreeMobileTarget(parsed)
	},
	"gchat": func(parsed *ParsedURL) (Sender, error) {
		return NewGoogleChatTarget(parsed)
	},
	"gio": func(parsed *ParsedURL) (Sender, error) {
		return NewLocalNotifyTarget(parsed)
	},
	"glib": func(parsed *ParsedURL) (Sender, error) {
		return NewLocalNotifyTarget(parsed)
	},
	"gnome": func(parsed *ParsedURL) (Sender, error) {
		return NewLocalNotifyTarget(parsed)
	},
	"gotify": func(parsed *ParsedURL) (Sender, error) {
		return NewGotifyTarget(parsed)
	},
	"gotifys": func(parsed *ParsedURL) (Sender, error) {
		return NewGotifyTarget(parsed)
	},
	"growl": func(parsed *ParsedURL) (Sender, error) {
		return NewGrowlTarget(parsed)
	},
	"guilded": func(parsed *ParsedURL) (Sender, error) {
		return NewGuildedTarget(parsed)
	},
	"hassio": func(parsed *ParsedURL) (Sender, error) {
		return NewHomeAssistantTarget(parsed)
	},
	"hassios": func(parsed *ParsedURL) (Sender, error) {
		return NewHomeAssistantTarget(parsed)
	},
	"httpsms": func(parsed *ParsedURL) (Sender, error) {
		return NewHttpSMSTarget(parsed)
	},
	"ifttt": func(parsed *ParsedURL) (Sender, error) {
		return NewIFTTTTarget(parsed)
	},
	"join": func(parsed *ParsedURL) (Sender, error) {
		return NewJoinTarget(parsed)
	},
	"json": func(parsed *ParsedURL) (Sender, error) {
		return NewJSONTarget(parsed)
	},
	"jsons": func(parsed *ParsedURL) (Sender, error) {
		return NewJSONTarget(parsed)
	},
	"kavenegar": func(parsed *ParsedURL) (Sender, error) {
		return NewKavenegarTarget(parsed)
	},
	"kde": func(parsed *ParsedURL) (Sender, error) {
		return NewLocalNotifyTarget(parsed)
	},
	"kodi": func(parsed *ParsedURL) (Sender, error) {
		return NewXBMCTarget(parsed)
	},
	"kodis": func(parsed *ParsedURL) (Sender, error) {
		return NewXBMCTarget(parsed)
	},
	"kumulos": func(parsed *ParsedURL) (Sender, error) {
		return NewKumulosTarget(parsed)
	},
	"lametric": func(parsed *ParsedURL) (Sender, error) {
		return NewLametricTarget(parsed)
	},
	"lametrics": func(parsed *ParsedURL) (Sender, error) {
		return NewLametricTarget(parsed)
	},
	"lark": func(parsed *ParsedURL) (Sender, error) {
		return NewLarkTarget(parsed)
	},
	"line": func(parsed *ParsedURL) (Sender, error) {
		return NewLineTarget(parsed)
	},
	"macosx": func(parsed *ParsedURL) (Sender, error) {
		return NewMacOSXTarget(parsed)
	},
	"mailgun": func(parsed *ParsedURL) (Sender, error) {
		return NewMailgunTarget(parsed)
	},
	"mailto": func(parsed *ParsedURL) (Sender, error) {
		return NewMailtoTarget(parsed)
	},
	"mailtos": func(parsed *ParsedURL) (Sender, error) {
		return NewMailtoTarget(parsed)
	},
	"mastodon": func(parsed *ParsedURL) (Sender, error) {
		return NewMastodonTarget(parsed)
	},
	"mastodons": func(parsed *ParsedURL) (Sender, error) {
		return NewMastodonTarget(parsed)
	},
	"matrix": func(parsed *ParsedURL) (Sender, error) {
		return NewMatrixTarget(parsed)
	},
	"matrixs": func(parsed *ParsedURL) (Sender, error) {
		return NewMatrixTarget(parsed)
	},
	"misskey": func(parsed *ParsedURL) (Sender, error) {
		return NewMisskeyTarget(parsed)
	},
	"misskeys": func(parsed *ParsedURL) (Sender, error) {
		return NewMisskeyTarget(parsed)
	},
	"mmost": func(parsed *ParsedURL) (Sender, error) {
		return NewMattermostTarget(parsed)
	},
	"mmosts": func(parsed *ParsedURL) (Sender, error) {
		return NewMattermostTarget(parsed)
	},
	"mqtt": func(parsed *ParsedURL) (Sender, error) {
		return NewMQTTTarget(parsed)
	},
	"mqtts": func(parsed *ParsedURL) (Sender, error) {
		return NewMQTTTarget(parsed)
	},
	"msg91": func(parsed *ParsedURL) (Sender, error) {
		return NewMSG91Target(parsed)
	},
	"msgbird": func(parsed *ParsedURL) (Sender, error) {
		return NewMessageBirdTarget(parsed)
	},
	"msteams": func(parsed *ParsedURL) (Sender, error) {
		return NewMSTeamsTarget(parsed)
	},
	"napi": func(parsed *ParsedURL) (Sender, error) {
		return NewNotificationAPITarget(parsed)
	},
	"ncloud": func(parsed *ParsedURL) (Sender, error) {
		return NewNextcloudTarget(parsed)
	},
	"nclouds": func(parsed *ParsedURL) (Sender, error) {
		return NewNextcloudTarget(parsed)
	},
	"nctalk": func(parsed *ParsedURL) (Sender, error) {
		return NewNextcloudTalkTarget(parsed)
	},
	"nctalks": func(parsed *ParsedURL) (Sender, error) {
		return NewNextcloudTalkTarget(parsed)
	},
	"nexmo": func(parsed *ParsedURL) (Sender, error) {
		return NewVonageTarget(parsed)
	},
	"notica": func(parsed *ParsedURL) (Sender, error) {
		return NewNoticaTarget(parsed)
	},
	"noticas": func(parsed *ParsedURL) (Sender, error) {
		return NewNoticaTarget(parsed)
	},
	"notifiarr": func(parsed *ParsedURL) (Sender, error) {
		return NewNotifiarrTarget(parsed)
	},
	"notificationapi": func(parsed *ParsedURL) (Sender, error) {
		return NewNotificationAPITarget(parsed)
	},
	"notifico": func(parsed *ParsedURL) (Sender, error) {
		return NewNotificoTarget(parsed)
	},
	"ntfy": func(parsed *ParsedURL) (Sender, error) {
		return NewNtfyTarget(parsed)
	},
	"ntfys": func(parsed *ParsedURL) (Sender, error) {
		return NewNtfyTarget(parsed)
	},
	"o365": func(parsed *ParsedURL) (Sender, error) {
		return NewOffice365Target(parsed)
	},
	"onesignal": func(parsed *ParsedURL) (Sender, error) {
		return NewOneSignalTarget(parsed)
	},
	"opsgenie": func(parsed *ParsedURL) (Sender, error) {
		return NewOpsgenieTarget(parsed)
	},
	"pagerduty": func(parsed *ParsedURL) (Sender, error) {
		return NewPagerDutyTarget(parsed)
	},
	"pagertree": func(parsed *ParsedURL) (Sender, error) {
		return NewPagerTreeTarget(parsed)
	},
	"parsep": func(parsed *ParsedURL) (Sender, error) {
		return NewParsePlatformTarget(parsed)
	},
	"parseps": func(parsed *ParsedURL) (Sender, error) {
		return NewParsePlatformTarget(parsed)
	},
	"pbul": func(parsed *ParsedURL) (Sender, error) {
		return NewPushbulletTarget(parsed)
	},
	"pjet": func(parsed *ParsedURL) (Sender, error) {
		return NewPushjetTarget(parsed)
	},
	"pjets": func(parsed *ParsedURL) (Sender, error) {
		return NewPushjetTarget(parsed)
	},
	"plivo": func(parsed *ParsedURL) (Sender, error) {
		return NewPlivoTarget(parsed)
	},
	"popcorn": func(parsed *ParsedURL) (Sender, error) {
		return NewPopcornTarget(parsed)
	},
	"pover": func(parsed *ParsedURL) (Sender, error) {
		return NewPushoverTarget(parsed)
	},
	"prowl": func(parsed *ParsedURL) (Sender, error) {
		return NewProwlTarget(parsed)
	},
	"psafer": func(parsed *ParsedURL) (Sender, error) {
		return NewPushSaferTarget(parsed)
	},
	"psafers": func(parsed *ParsedURL) (Sender, error) {
		return NewPushSaferTarget(parsed)
	},
	"push": func(parsed *ParsedURL) (Sender, error) {
		return NewTechulusPushTarget(parsed)
	},
	"pushdeer": func(parsed *ParsedURL) (Sender, error) {
		return NewPushDeerTarget(parsed)
	},
	"pushdeers": func(parsed *ParsedURL) (Sender, error) {
		return NewPushDeerTarget(parsed)
	},
	"pushed": func(parsed *ParsedURL) (Sender, error) {
		return NewPushedTarget(parsed)
	},
	"pushme": func(parsed *ParsedURL) (Sender, error) {
		return NewPushMeTarget(parsed)
	},
	"pushplus": func(parsed *ParsedURL) (Sender, error) {
		return NewPushplusTarget(parsed)
	},
	"pushy": func(parsed *ParsedURL) (Sender, error) {
		return NewPushyTarget(parsed)
	},
	"qq": func(parsed *ParsedURL) (Sender, error) {
		return NewQQTarget(parsed)
	},
	"qt": func(parsed *ParsedURL) (Sender, error) {
		return NewLocalNotifyTarget(parsed)
	},
	"reddit": func(parsed *ParsedURL) (Sender, error) {
		return NewRedditTarget(parsed)
	},
	"resend": func(parsed *ParsedURL) (Sender, error) {
		return NewResendTarget(parsed)
	},
	"revolt": func(parsed *ParsedURL) (Sender, error) {
		return NewRevoltTarget(parsed)
	},
	"rocket": func(parsed *ParsedURL) (Sender, error) {
		return NewRocketChatTarget(parsed)
	},
	"rockets": func(parsed *ParsedURL) (Sender, error) {
		return NewRocketChatTarget(parsed)
	},
	"rsyslog": func(parsed *ParsedURL) (Sender, error) {
		return NewRSyslogTarget(parsed)
	},
	"ryver": func(parsed *ParsedURL) (Sender, error) {
		return NewRyverTarget(parsed)
	},
	"schan": func(parsed *ParsedURL) (Sender, error) {
		return NewServerChanTarget(parsed)
	},
	"sendgrid": func(parsed *ParsedURL) (Sender, error) {
		return NewSendGridTarget(parsed)
	},
	"sendpulse": func(parsed *ParsedURL) (Sender, error) {
		return NewSendPulseTarget(parsed)
	},
	"ses": func(parsed *ParsedURL) (Sender, error) {
		return NewSESTarget(parsed)
	},
	"seven": func(parsed *ParsedURL) (Sender, error) {
		return NewSevenTarget(parsed)
	},
	"sfr": func(parsed *ParsedURL) (Sender, error) {
		return NewSFRTarget(parsed)
	},
	"signal": func(parsed *ParsedURL) (Sender, error) {
		return NewSignalTarget(parsed)
	},
	"signals": func(parsed *ParsedURL) (Sender, error) {
		return NewSignalTarget(parsed)
	},
	"signl4": func(parsed *ParsedURL) (Sender, error) {
		return NewSignl4Target(parsed)
	},
	"sinch": func(parsed *ParsedURL) (Sender, error) {
		return NewSinchTarget(parsed)
	},
	"slack": func(parsed *ParsedURL) (Sender, error) {
		return NewSlackTarget(parsed)
	},
	"smpp": func(parsed *ParsedURL) (Sender, error) {
		return NewSMPPTarget(parsed)
	},
	"smpps": func(parsed *ParsedURL) (Sender, error) {
		return NewSMPPTarget(parsed)
	},
	"smseagle": func(parsed *ParsedURL) (Sender, error) {
		return NewSMSEagleTarget(parsed)
	},
	"smseagles": func(parsed *ParsedURL) (Sender, error) {
		return NewSMSEagleTarget(parsed)
	},
	"smsmanager": func(parsed *ParsedURL) (Sender, error) {
		return NewSMSManagerTarget(parsed)
	},
	"smsmgr": func(parsed *ParsedURL) (Sender, error) {
		return NewSMSManagerTarget(parsed)
	},
	"smtp2go": func(parsed *ParsedURL) (Sender, error) {
		return NewSMTP2GoTarget(parsed)
	},
	"sns": func(parsed *ParsedURL) (Sender, error) {
		return NewSNSTarget(parsed)
	},
	"sparkpost": func(parsed *ParsedURL) (Sender, error) {
		return NewSparkPostTarget(parsed)
	},
	"spike": func(parsed *ParsedURL) (Sender, error) {
		return NewSpikeTarget(parsed)
	},
	"splunk": func(parsed *ParsedURL) (Sender, error) {
		return NewSplunkTarget(parsed)
	},
	"spugpush": func(parsed *ParsedURL) (Sender, error) {
		return NewSpugpushTarget(parsed)
	},
	"spush": func(parsed *ParsedURL) (Sender, error) {
		return NewSimplePushTarget(parsed)
	},
	"strmlabs": func(parsed *ParsedURL) (Sender, error) {
		return NewStreamlabsTarget(parsed)
	},
	"synology": func(parsed *ParsedURL) (Sender, error) {
		return NewSynologyTarget(parsed)
	},
	"synologys": func(parsed *ParsedURL) (Sender, error) {
		return NewSynologyTarget(parsed)
	},
	"syslog": func(parsed *ParsedURL) (Sender, error) {
		return NewSyslogTarget(parsed)
	},
	"tgram": func(parsed *ParsedURL) (Sender, error) {
		return NewTelegramTarget(parsed)
	},
	"threema": func(parsed *ParsedURL) (Sender, error) {
		return NewThreemaTarget(parsed)
	},
	"toot": func(parsed *ParsedURL) (Sender, error) {
		return NewMastodonTarget(parsed)
	},
	"toots": func(parsed *ParsedURL) (Sender, error) {
		return NewMastodonTarget(parsed)
	},
	"tweet": func(parsed *ParsedURL) (Sender, error) {
		return NewTwitterTarget(parsed)
	},
	"twilio": func(parsed *ParsedURL) (Sender, error) {
		return NewTwilioTarget(parsed)
	},
	"twist": func(parsed *ParsedURL) (Sender, error) {
		return NewTwistTarget(parsed)
	},
	"twitter": func(parsed *ParsedURL) (Sender, error) {
		return NewTwitterTarget(parsed)
	},
	"vapid": func(parsed *ParsedURL) (Sender, error) {
		return NewVapidTarget(parsed)
	},
	"victorops": func(parsed *ParsedURL) (Sender, error) {
		return NewSplunkTarget(parsed)
	},
	"voipms": func(parsed *ParsedURL) (Sender, error) {
		return NewVoipmsTarget(parsed)
	},
	"vonage": func(parsed *ParsedURL) (Sender, error) {
		return NewVonageTarget(parsed)
	},
	"webex": func(parsed *ParsedURL) (Sender, error) {
		return NewWebexTeamsTarget(parsed)
	},
	"wecombot": func(parsed *ParsedURL) (Sender, error) {
		return NewWeComBotTarget(parsed)
	},
	"whatsapp": func(parsed *ParsedURL) (Sender, error) {
		return NewWhatsAppTarget(parsed)
	},
	"windows": func(parsed *ParsedURL) (Sender, error) {
		return NewWindowsTarget(parsed)
	},
	"workflow": func(parsed *ParsedURL) (Sender, error) {
		return NewWorkflowsTarget(parsed)
	},
	"workflows": func(parsed *ParsedURL) (Sender, error) {
		return NewWorkflowsTarget(parsed)
	},
	"wxpusher": func(parsed *ParsedURL) (Sender, error) {
		return NewWxPusherTarget(parsed)
	},
	"wxteams": func(parsed *ParsedURL) (Sender, error) {
		return NewWebexTeamsTarget(parsed)
	},
	"x": func(parsed *ParsedURL) (Sender, error) {
		return NewTwitterTarget(parsed)
	},
	"xbmc": func(parsed *ParsedURL) (Sender, error) {
		return NewXBMCTarget(parsed)
	},
	"xbmcs": func(parsed *ParsedURL) (Sender, error) {
		return NewXBMCTarget(parsed)
	},
	"xml": func(parsed *ParsedURL) (Sender, error) {
		return NewXMLTarget(parsed)
	},
	"xmls": func(parsed *ParsedURL) (Sender, error) {
		return NewXMLTarget(parsed)
	},
	"zulip": func(parsed *ParsedURL) (Sender, error) {
		return NewZulipTarget(parsed)
	},
}
