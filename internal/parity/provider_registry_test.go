package parity

import "github.com/unraid/apprise-go/internal/notify"

type requestSender interface {
	Send(body, title string, notifyType notify.NotifyType) error
}

type buildTargetFunc func(parsed *notify.ParsedURL) (requestSender, error)

var providerBuilders = map[string]buildTargetFunc{
	"apprise": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewAppriseTarget(parsed)
	},
	"discord": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewDiscordTarget(parsed)
	},
	"bark": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewBarkTarget(parsed)
	},
	"freemobile": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewFreeMobileTarget(parsed)
	},
	"gchat": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewGoogleChatTarget(parsed)
	},
	"feishu": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewFeishuTarget(parsed)
	},
	"lark": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewLarkTarget(parsed)
	},
	"webex": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewWebexTeamsTarget(parsed)
	},
	"line": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewLineTarget(parsed)
	},
	"guilded": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewGuildedTarget(parsed)
	},
	"dot": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewDotTarget(parsed)
	},
	"splunk": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSplunkTarget(parsed)
	},
	"workflow": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewWorkflowsTarget(parsed)
	},
	"flock": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewFlockTarget(parsed)
	},
	"hassio": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewHomeAssistantTarget(parsed)
	},
	"emby": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewEmbyTarget(parsed)
	},
	"kodi": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewXBMCTarget(parsed)
	},
	"kumulos": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewKumulosTarget(parsed)
	},
	"nctalk": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewNextcloudTalkTarget(parsed)
	},
	"popcorn": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPopcornTarget(parsed)
	},
	"httpsms": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewHttpSMSTarget(parsed)
	},
	"d7sms": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewD7NetworksTarget(parsed)
	},
	"atalk": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewAfricasTalkingTarget(parsed)
	},
	"kavenegar": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewKavenegarTarget(parsed)
	},
	"clickatell": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewClickatellTarget(parsed)
	},
	"clicksend": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewClickSendTarget(parsed)
	},
	"46elks": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewFortySixElksTarget(parsed)
	},
	"seven": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSevenTarget(parsed)
	},
	"msgbird": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewMessageBirdTarget(parsed)
	},
	"msg91": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewMSG91Target(parsed)
	},
	"plivo": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPlivoTarget(parsed)
	},
	"vonage": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewVonageTarget(parsed)
	},
	"twilio": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewTwilioTarget(parsed)
	},
	"bulkvs": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewBulkVSTarget(parsed)
	},
	"bulksms": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewBulkSMSTarget(parsed)
	},
	"burstsms": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewBurstSMSTarget(parsed)
	},
	"smseagle": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSMSEagleTarget(parsed)
	},
	"smsmanager": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSMSManagerTarget(parsed)
	},
	"sfr": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSFRTarget(parsed)
	},
	"voipms": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewVoipmsTarget(parsed)
	},
	"sinch": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSinchTarget(parsed)
	},
	"signal": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSignalTarget(parsed)
	},
	"whatsapp": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewWhatsAppTarget(parsed)
	},
	"ryver": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewRyverTarget(parsed)
	},
	"zulip": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewZulipTarget(parsed)
	},
	"rocket": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewRocketChatTarget(parsed)
	},
	"slack": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSlackTarget(parsed)
	},
	"msteams": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewMSTeamsTarget(parsed)
	},
	"revolt": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewRevoltTarget(parsed)
	},
	"mmost": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewMattermostTarget(parsed)
	},
	"dingtalk": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewDingTalkTarget(parsed)
	},
	"join": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewJoinTarget(parsed)
	},
	"pagertree": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPagerTreeTarget(parsed)
	},
	"pagerduty": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPagerDutyTarget(parsed)
	},
	"opsgenie": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewOpsgenieTarget(parsed)
	},
	"matrix": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewMatrixTarget(parsed)
	},
	"wecombot": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewWeComBotTarget(parsed)
	},
	"chanify": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewChanifyTarget(parsed)
	},
	"form": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewFormTarget(parsed)
	},
	"gotify": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewGotifyTarget(parsed)
	},
	"fcm": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewFCMTarget(parsed)
	},
	"ifttt": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewIFTTTTarget(parsed)
	},
	"json": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewJSONTarget(parsed)
	},
	"misskey": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewMisskeyTarget(parsed)
	},
	"notica": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewNoticaTarget(parsed)
	},
	"notifico": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewNotificoTarget(parsed)
	},
	"ntfy": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewNtfyTarget(parsed)
	},
	"reddit": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewRedditTarget(parsed)
	},
	"office365": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewOffice365Target(parsed)
	},
	"ses": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSESTarget(parsed)
	},
	"sns": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSNSTarget(parsed)
	},
	"twitter": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewTwitterTarget(parsed)
	},
	"twist": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewTwistTarget(parsed)
	},
	"vapid": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewVapidTarget(parsed)
	},
	"prowl": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewProwlTarget(parsed)
	},
	"push": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewTechulusPushTarget(parsed)
	},
	"pushbullet": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPushbulletTarget(parsed)
	},
	"pushdeer": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPushDeerTarget(parsed)
	},
	"pushed": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPushedTarget(parsed)
	},
	"pushjet": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPushjetTarget(parsed)
	},
	"pushme": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPushMeTarget(parsed)
	},
	"pushplus": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPushplusTarget(parsed)
	},
	"smtp2go": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSMTP2GoTarget(parsed)
	},
	"sendpulse": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSendPulseTarget(parsed)
	},
	"sendgrid": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSendGridTarget(parsed)
	},
	"sparkpost": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSparkPostTarget(parsed)
	},
	"resend": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewResendTarget(parsed)
	},
	"brevo": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewBrevoTarget(parsed)
	},
	"mailgun": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewMailgunTarget(parsed)
	},
	"pushy": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPushyTarget(parsed)
	},
	"pushover": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPushoverTarget(parsed)
	},
	"qq": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewQQTarget(parsed)
	},
	"schan": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewServerChanTarget(parsed)
	},
	"dapnet": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewDapnetTarget(parsed)
	},
	"enigma2": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewEnigma2Target(parsed)
	},
	"notifiarr": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewNotifiarrTarget(parsed)
	},
	"notificationapi": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewNotificationAPITarget(parsed)
	},
	"onesignal": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewOneSignalTarget(parsed)
	},
	"parsep": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewParsePlatformTarget(parsed)
	},
	"wxpusher": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewWxPusherTarget(parsed)
	},
	"spike": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSpikeTarget(parsed)
	},
	"synology": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSynologyTarget(parsed)
	},
	"strmlabs": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewStreamlabsTarget(parsed)
	},
	"threema": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewThreemaTarget(parsed)
	},
	"psafer": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewPushSaferTarget(parsed)
	},
	"tgram": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewTelegramTarget(parsed)
	},
	"lametric": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewLametricTarget(parsed)
	},
	"bluesky": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewBlueskyTarget(parsed)
	},
	"ncloud": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewNextcloudTarget(parsed)
	},
	"mastodon": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewMastodonTarget(parsed)
	},
	"signl4": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSignl4Target(parsed)
	},
	"spugpush": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSpugpushTarget(parsed)
	},
	"spush": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewSimplePushTarget(parsed)
	},
	"xml": func(parsed *notify.ParsedURL) (requestSender, error) {
		return notify.NewXMLTarget(parsed)
	},
}
