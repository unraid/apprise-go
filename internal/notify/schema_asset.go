package notify

func init() {
	RegisterSchemaAsset(map[string]any{
		"app_desc":          "Apprise Notifications",
		"app_id":            "Apprise",
		"default_extension": ".png",
		"image_path_mask":   "apprise/assets/themes/{THEME}/apprise-{TYPE}-{XY}{EXTENSION}",
		"image_url_logo":    "https://github.com/caronc/apprise/raw/master/apprise/assets/themes/{THEME}/apprise-logo.png",
		"image_url_mask":    "https://github.com/caronc/apprise/raw/master/apprise/assets/themes/{THEME}/apprise-{TYPE}-{XY}{EXTENSION}",
		"theme":             "default",
	})
}
