// RotomNG is a distributed MITM proxy system.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/UnownHash/RotomNG/libs/gitutil"
	"github.com/UnownHash/RotomNG/libs/rotom_ui"

	rotomapp "github.com/UnownHash/RotomNG/apps/rotom-ng/app"
	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/version"
)

const defaultConfigFile = "configs/rotom-ng.toml"

func main() {
	var flagCfg rotomapp.FlagConfig
	uiFS := rotom_ui.GetUIFS()
	hasEmbeddedUI := uiFS != nil

	// Parse command line flags
	flag.BoolVar(&flagCfg.DebugMode, "debug", false, "Enable debug mode (sets Gin to debug mode)")
	if hasEmbeddedUI {
		flag.StringVar(&flagCfg.UIPath, "ui-path", "", "Path to the UI static files directory to override embedded UI")
	} else {
		flag.StringVar(&flagCfg.UIPath, "ui-path", "./libs/rotom_ui/static", "Path to the UI static files directory")
	}
	flag.BoolVar(&flagCfg.UIDev, "ui-dev", false, "Enable UI development mode (proxy to dev server)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		log.Printf("RotomNG %s[%s]", version.AppVersion, gitutil.GetGitBuildSHA())
		os.Exit(0)
	}

	flagCfg.UIFS = uiFS

	// Load config - use remaining args for config path
	configPath := defaultConfigFile
	args := flag.Args()
	if len(args) > 0 {
		configPath = args[0]
	}

	flagCfg.ReloadConfig = func() (*config.Config, error) {
		return config.LoadFromFile(configPath)
	}

	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		log.Fatalf("Failed to load config from '%s': %v", configPath, err)
	}

	app, err := rotomapp.NewApp(cfg, flagCfg)
	if err != nil {
		log.Fatalf("Failed to create app: %v", err)
	}

	if err := app.Init(); err != nil {
		log.Fatalf("failed to initialize app: %v", err)
	}

	app.Run()
}
