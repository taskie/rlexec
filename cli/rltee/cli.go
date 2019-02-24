package rltee

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/chzyer/readline"
	"github.com/iancoleman/strcase"

	"github.com/k0kubun/pp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/taskie/osplus"
	"github.com/taskie/rltee"
)

type Config struct {
	History, LogLevel string
	Buffered, Temp    bool
}

var configFile string
var config Config
var (
	verbose, debug, version bool
)

const CommandName = "rltee"

func init() {
	Command.PersistentFlags().StringVarP(&configFile, "config", "c", "", `config file (default "`+CommandName+`.yml")`)
	Command.Flags().StringP("history", "H", "", "history file")
	Command.Flags().StringP("buffered", "b", "", "use buffer for output")
	Command.Flags().StringP("temp", "t", "", "use tempfile for output")
	Command.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	Command.Flags().BoolVar(&debug, "debug", false, "debug output")
	Command.Flags().BoolVarP(&version, "version", "V", false, "show Version")

	for _, s := range []string{"history", "buffered", "temp"} {
		envKey := strcase.ToSnake(s)
		structKey := strcase.ToCamel(s)
		viper.BindPFlag(envKey, Command.Flags().Lookup(s))
		viper.RegisterAlias(structKey, envKey)
	}

	cobra.OnInitialize(initConfig)
}

func initConfig() {
	if debug {
		log.SetLevel(log.DebugLevel)
	} else if verbose {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName(CommandName)
		conf, err := osplus.GetXdgConfigHome()
		if err != nil {
			log.Info(err)
		} else {
			viper.AddConfigPath(filepath.Join(conf, CommandName))
		}
		viper.AddConfigPath(".")
	}
	viper.SetEnvPrefix(CommandName)
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		log.Debug(err)
	}
	err = viper.Unmarshal(&config)
	if err != nil {
		log.Warn(err)
	}
}

func Main() {
	Command.Execute()
}

var Command = &cobra.Command{
	Use:  CommandName + ` [OUTPUT]`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		err := run(cmd, args)
		if err != nil {
			log.Fatal(err)
		}
	},
}

func run(cmd *cobra.Command, args []string) error {
	if version {
		fmt.Println(rltee.Version)
		return nil
	}
	if config.LogLevel != "" {
		lv, err := log.ParseLevel(config.LogLevel)
		if err != nil {
			log.Warn(err)
		} else {
			log.SetLevel(lv)
		}
	}
	if debug {
		if viper.ConfigFileUsed() != "" {
			log.Debugf("Using config file: %s", viper.ConfigFileUsed())
		}
		log.Debug(pp.Sprint(config))
	}

	output := args[0]

	opener := osplus.NewOpener()
	opener.Unbuffered = !config.Buffered
	var w io.WriteCloser
	var err error
	commit := func(bool) {}
	if config.Temp {
		w, commit, err = opener.CreateTempFileWithDestination(output, "", CommandName+"-")
		if err != nil {
			return err
		}
		defer w.Close()
	} else {
		w, err = opener.Create(output)
		if err != nil {
			return err
		}
		defer w.Close()
	}
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "> ",
		HistoryFile:     config.History,
		InterruptPrompt: "^C",
	})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}
		fmt.Fprintln(w, line)
		err = rl.SaveHistory(line)
		if err != nil {
			log.Warn(err)
		}
	}

	commit(true)
	return nil
}
