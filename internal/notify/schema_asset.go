package notify

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const defaultImagePathMask = "apprise/assets/themes/{THEME}/apprise-{TYPE}-{XY}{EXTENSION}"

var (
	imagePathMaskOnce sync.Once
	imagePathMask     string
)

func init() {
	RegisterSchemaAsset(map[string]any{
		"app_desc":          "Apprise Notifications",
		"app_id":            "Apprise",
		"default_extension": ".png",
		"image_path_mask":   defaultImagePathMask,
		"image_url_logo":    "https://github.com/caronc/apprise/raw/master/apprise/assets/themes/{THEME}/apprise-logo.png",
		"image_url_mask":    "https://github.com/caronc/apprise/raw/master/apprise/assets/themes/{THEME}/apprise-{TYPE}-{XY}{EXTENSION}",
		"theme":             "default",
	})
}

func resolveImagePathMask() string {
	imagePathMaskOnce.Do(func() {
		imagePathMask = computeImagePathMask()
	})
	return imagePathMask
}

func computeImagePathMask() string {
	root := strings.TrimSpace(os.Getenv("APPRISE_ASSET_ROOT"))
	if root != "" {
		return joinAssetMask(root)
	}

	if moduleRoot, ok := findModuleRoot(); ok {
		candidateRoot := filepath.Join(moduleRoot, "..", "apprise", "apprise")
		if dirExists(filepath.Join(candidateRoot, "assets")) {
			return joinAssetMask(candidateRoot)
		}
	}

	return defaultImagePathMask
}

func joinAssetMask(root string) string {
	root = filepath.Clean(root)
	if strings.HasSuffix(root, string(filepath.Separator)+"assets") {
		return filepath.Join(root, "themes", "{THEME}", "apprise-{TYPE}-{XY}{EXTENSION}")
	}
	return filepath.Join(root, "assets", "themes", "{THEME}", "apprise-{TYPE}-{XY}{EXTENSION}")
}

func findModuleRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}

	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
