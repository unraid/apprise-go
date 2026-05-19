package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/version"
)

const usageText = "" +
	"Usage:\n" +
	"   apprise [OPTIONS] [APPRISE_URL [APPRISE_URL2 [APPRISE_URL3]]]\n" +
	"   apprise storage [OPTIONS] [ACTION] [UID1 [UID2 [UID3]]]\n"

type cliOptions struct {
	body             string
	title            string
	notificationType string
	inputFormat      string
	disableAsync     bool
	showVersion      bool
	showHelp         bool
	showSchema       bool
	showDetails      bool
	dryRun           bool
	debug            bool
	verbose          int
	recursionDepth   int
	interpretEscapes bool
	interpretEmojis  bool
	theme            string
	configPaths      []string
	attachments      []string
	pluginPaths      []string
	tags             []string
	storagePath      string
	storagePruneDays int
	storageUIDLength int
	storageMode      string
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type countFlag int

func (c *countFlag) String() string {
	return strconv.Itoa(int(*c))
}

func (c *countFlag) Set(value string) error {
	*c++
	return nil
}

func (c *countFlag) IsBoolFlag() bool {
	return true
}

func Run(args []string, stdout, stderr io.Writer) int {
	opts := defaultCliOptions()
	args = normalizeArgs(args)
	fs := flag.NewFlagSet("apprise", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.StringVar(&opts.body, "body", "", "Specify the message body.")
	fs.StringVar(&opts.body, "b", "", "Specify the message body.")
	fs.StringVar(&opts.title, "title", "", "Specify the message title.")
	fs.StringVar(&opts.title, "t", "", "Specify the message title.")
	fs.StringVar(&opts.notificationType, "notification-type", opts.notificationType, "Specify the message type.")
	fs.StringVar(&opts.notificationType, "n", opts.notificationType, "Specify the message type.")
	fs.StringVar(&opts.inputFormat, "input-format", opts.inputFormat, "Specify the message input format.")
	fs.StringVar(&opts.inputFormat, "i", opts.inputFormat, "Specify the message input format.")
	fs.BoolVar(&opts.disableAsync, "disable-async", false, "Send all notifications sequentially.")
	fs.BoolVar(&opts.disableAsync, "Da", false, "Send all notifications sequentially.")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "Perform a trial run without sending notifications.")
	fs.BoolVar(&opts.dryRun, "d", false, "Perform a trial run without sending notifications.")
	fs.BoolVar(&opts.showDetails, "details", false, "Prints details about the current services supported by Apprise.")
	fs.BoolVar(&opts.showDetails, "l", false, "Prints details about the current services supported by Apprise.")
	fs.BoolVar(&opts.showSchema, "schema", false, "Prints Apprise schema JSON and exits.")
	fs.IntVar(&opts.recursionDepth, "recursion-depth", opts.recursionDepth, "Specify the recursion depth when loading configs.")
	fs.IntVar(&opts.recursionDepth, "R", opts.recursionDepth, "Specify the recursion depth when loading configs.")
	fs.Var((*countFlag)(&opts.verbose), "v", "Increase verbosity.")
	fs.Var((*countFlag)(&opts.verbose), "verbose", "Increase verbosity.")
	fs.BoolVar(&opts.interpretEscapes, "interpret-escapes", false, "Enable interpretation of backslash escapes.")
	fs.BoolVar(&opts.interpretEscapes, "e", false, "Enable interpretation of backslash escapes.")
	fs.BoolVar(&opts.interpretEmojis, "interpret-emojis", false, "Enable interpretation of :emoji: definitions.")
	fs.BoolVar(&opts.interpretEmojis, "j", false, "Enable interpretation of :emoji: definitions.")
	fs.BoolVar(&opts.debug, "debug", false, "Debug mode.")
	fs.BoolVar(&opts.debug, "D", false, "Debug mode.")
	fs.StringVar(&opts.theme, "theme", opts.theme, "Specify the default theme.")
	fs.StringVar(&opts.theme, "T", opts.theme, "Specify the default theme.")
	fs.Var((*stringSliceFlag)(&opts.tags), "tag", "Specify tags used to filter which services to notify.")
	fs.Var((*stringSliceFlag)(&opts.tags), "g", "Specify tags used to filter which services to notify.")
	fs.Var((*stringSliceFlag)(&opts.configPaths), "config", "Specify one or more configuration locations.")
	fs.Var((*stringSliceFlag)(&opts.configPaths), "c", "Specify one or more configuration locations.")
	fs.Var((*stringSliceFlag)(&opts.attachments), "attach", "Specify one or more attachments.")
	fs.Var((*stringSliceFlag)(&opts.attachments), "a", "Specify one or more attachments.")
	fs.Var((*stringSliceFlag)(&opts.pluginPaths), "plugin-path", "Specify one or more plugin paths to scan.")
	fs.Var((*stringSliceFlag)(&opts.pluginPaths), "P", "Specify one or more plugin paths to scan.")
	fs.StringVar(&opts.storagePath, "storage-path", opts.storagePath, "Specify the path to the persistent storage location.")
	fs.StringVar(&opts.storagePath, "S", opts.storagePath, "Specify the path to the persistent storage location.")
	fs.IntVar(&opts.storagePruneDays, "storage-prune-days", opts.storagePruneDays, "Define the number of days the storage prune should run using.")
	fs.IntVar(&opts.storagePruneDays, "SPD", opts.storagePruneDays, "Define the number of days the storage prune should run using.")
	fs.IntVar(&opts.storageUIDLength, "storage-uid-length", opts.storageUIDLength, "Define the number of unique characters to store persistent cache in.")
	fs.IntVar(&opts.storageUIDLength, "SUL", opts.storageUIDLength, "Define the number of unique characters to store persistent cache in.")
	fs.StringVar(&opts.storageMode, "storage-mode", opts.storageMode, "Specify the persistent storage operational mode.")
	fs.StringVar(&opts.storageMode, "SM", opts.storageMode, "Specify the persistent storage operational mode.")
	fs.BoolVar(&opts.showVersion, "version", false, "Display the apprise version and exit.")
	fs.BoolVar(&opts.showVersion, "V", false, "Display the apprise version and exit.")
	fs.BoolVar(&opts.showHelp, "help", false, "Show help.")
	fs.BoolVar(&opts.showHelp, "h", false, "Show help.")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return 0
		}
		fmt.Fprintln(stderr, err)
		printUsage(stderr)
		return 2
	}

	if opts.showHelp {
		printUsage(stdout)
		return 0
	}

	if opts.showVersion {
		fmt.Fprintln(stdout, version.Message())
		return 0
	}

	if opts.showSchema {
		schemaJSON, err := SchemaJSON()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if _, err := stdout.Write(append(schemaJSON, '\n')); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if opts.showDetails {
		if err := PrintDetails(stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	urls := fs.Args()
	if isStorageAction(urls) {
		return RunStorage(&opts, urls, stdout, stderr)
	}

	tagged := resolveNotifyURLs(&opts, urls, stderr)
	if len(tagged) == 0 {
		fmt.Fprintln(stdout, "You must specify at least one server URL or populated configuration file.")
		fmt.Fprintln(stdout, "Try 'apprise --help' for more information.")
		return 1
	}

	nt, ok := notify.ParseNotifyType(opts.notificationType)
	if !ok {
		fmt.Fprintf(stderr, "unsupported notification type: %s\n", opts.notificationType)
		return 2
	}

	body := opts.body
	title := opts.title
	if body == "" {
		data, err := io.ReadAll(os.Stdin)
		if err == nil {
			body = string(data)
		}
	}

	// TODO: Wire these options into CLI behavior once the runtime supports them.
	_ = opts.disableAsync
	_ = opts.attachments
	_ = opts.pluginPaths
	_ = opts.theme
	_ = opts.recursionDepth
	_ = opts.dryRun
	_ = opts.debug
	_ = opts.verbose
	_ = opts.interpretEscapes
	_ = opts.interpretEmojis
	_ = opts.storageMode
	_ = opts.storagePath
	_ = opts.storagePruneDays
	_ = opts.storageUIDLength

	failed := false
	for _, entry := range tagged {
		parsed, err := notify.ParseURL(entry.URL)
		if err != nil {
			fmt.Fprintf(stderr, "invalid url: %s\n", err)
			failed = true
			continue
		}

		sendBody, err := notify.ConvertMessageFormatForTarget(parsed, body, opts.inputFormat)
		if err != nil {
			fmt.Fprintln(stderr, err)
			failed = true
			continue
		}

		target, err := notify.NewTarget(parsed)
		if err != nil {
			if notify.IsUnsupportedSchema(err) {
				fmt.Fprintf(stderr, "unsupported url schema: %s\n", parsed.Scheme)
			} else {
				fmt.Fprintf(stderr, "%s target error: %s\n", notify.TargetSchemaName(parsed.Scheme), err)
			}
			failed = true
			continue
		}

		if err := target.Send(sendBody, title, nt); err != nil {
			fmt.Fprintf(stderr, "%s notify error: %s\n", notify.TargetSchemaName(parsed.Scheme), err)
			failed = true
		}
	}

	if failed {
		return 1
	}

	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, usageText)
}

func defaultCliOptions() cliOptions {
	return cliOptions{
		notificationType: string(notify.NotifyInfo),
		inputFormat:      "text",
		theme:            "default",
		recursionDepth:   1,
		storagePath:      defaultStoragePath,
		storageMode:      defaultStorageMode,
		storagePruneDays: envInt("APPRISE_STORAGE_PRUNE_DAYS", defaultStoragePruneDays),
		storageUIDLength: envInt("APPRISE_STORAGE_UID_LENGTH", defaultStorageUIDLength),
	}
}

func envInt(name string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return fallback
}

func normalizeArgs(args []string) []string {
	normalized := []string{}
	for _, arg := range args {
		if isVerboseBundle(arg) {
			for range strings.TrimPrefix(arg, "-") {
				normalized = append(normalized, "-v")
			}
			continue
		}
		normalized = append(normalized, arg)
	}
	return normalized
}

func isVerboseBundle(arg string) bool {
	if len(arg) < 3 || !strings.HasPrefix(arg, "-") {
		return false
	}
	trimmed := strings.TrimPrefix(arg, "-")
	for _, r := range trimmed {
		if r != 'v' {
			return false
		}
	}
	return true
}

func isStorageAction(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return strings.HasPrefix("storage", strings.ToLower(args[0]))
}

func resolveNotifyURLs(opts *cliOptions, args []string, stderr io.Writer) []taggedURL {
	if len(args) > 0 {
		if len(opts.tags) > 0 {
			fmt.Fprintln(stderr, "--tag (-g) entries are ignored when using specified URLs")
		}
		if len(opts.configPaths) > 0 {
			fmt.Fprintln(stderr, "You defined both URLs and a --config (-c) entry; Only the URLs will be referenced.")
		}

		var urls []taggedURL
		for _, arg := range args {
			for _, raw := range detectURLs(arg) {
				if strings.TrimSpace(raw) == "" {
					continue
				}
				urls = append(urls, taggedURL{URL: raw})
			}
		}
		return urls
	}

	if len(opts.configPaths) > 0 {
		return filterTaggedURLs(loadTaggedURLs(loadConfigPaths(opts.configPaths)), parseTagFilters(opts.tags))
	}

	if raw := strings.TrimSpace(os.Getenv(defaultEnvAppriseURLs)); raw != "" {
		parsed := parseTaggedLine(raw)
		if len(parsed) == 0 {
			return nil
		}
		return filterTaggedURLs(parsed, parseTagFilters(opts.tags))
	}

	return filterTaggedURLs(loadTaggedURLs(loadConfigPaths(nil)), parseTagFilters(opts.tags))
}
