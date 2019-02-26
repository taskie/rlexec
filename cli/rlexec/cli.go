package rlexec

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/chzyer/readline"
	"github.com/iancoleman/strcase"

	"github.com/k0kubun/pp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/taskie/osplus"
	"github.com/taskie/rlexec"
)

type Config struct {
	Output, History, Prompt, LogLevel string
	Buffered, Temp                    bool
}

var configFile string
var config Config
var (
	verbose, debug, version bool
)

const CommandName = "rlexec"

func init() {
	Command.PersistentFlags().StringVarP(&configFile, "config", "c", "", `config file (default "`+CommandName+`.yml")`)
	Command.Flags().SetInterspersed(false)
	Command.Flags().StringP("output", "o", "", "output file")
	Command.Flags().StringP("history", "H", "", "history file")
	Command.Flags().StringP("prompt", "p", "> ", "prompt")
	Command.Flags().StringP("buffered", "r", "", "use buffer for output")
	Command.Flags().StringP("temp", "t", "", "use tempfile for output")
	Command.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	Command.Flags().BoolVar(&debug, "debug", false, "debug output")
	Command.Flags().BoolVarP(&version, "version", "V", false, "show Version")

	for _, s := range []string{"output", "history", "prompt", "buffered", "temp"} {
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

var globalExitStatus int

var Command = &cobra.Command{
	Use:  CommandName + ` [OUTPUT]`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		err := run(cmd, args)
		if err != nil {
			log.Fatal(err)
		}
		if globalExitStatus != 0 {
			os.Exit(globalExitStatus)
		}
	},
}

func run(cmd *cobra.Command, args []string) error {
	if version {
		fmt.Println(rlexec.Version)
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

	output := config.Output

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
		Prompt:          config.Prompt,
		HistoryFile:     config.History,
		InterruptPrompt: "^C",
	})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	if len(args) > 0 {
		ctx := context.Background()
		globalExitStatus, err = execute(ctx, w, rl, args[0], args[1:])
		if err != nil {
			return err
		}
	} else {
		err = print(w, rl)
		if err != nil {
			return err
		}
	}

	commit(true)
	return nil
}

func print(w io.Writer, rl *readline.Instance) error {
	for {
		line, err := rl.Readline()
		if err != nil {
			return nil
		}
		_, err = fmt.Fprintln(w, line)
		if err != nil {
			return err
		}
		err = rl.SaveHistory(line)
		if err != nil {
			log.Warn(err)
		}
	}
}

func execute(ctx context.Context, w io.Writer, rl *readline.Instance, name string, args []string) (int, error) {
	log.Debugf("executing %s %v", name, args)
	cmdCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return 0, err
	}
	defer stdinPipe.Close()
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		return 0, err
	}
	for {
		line, err := rl.Readline()
		if err != nil {
			if err != io.EOF {
				log.Info(err)
			}
			stdinPipe.Close()
			err = cmd.Wait()
			if err2, ok := err.(*exec.ExitError); ok {
				if s, ok := err2.Sys().(syscall.WaitStatus); ok {
					return s.ExitStatus(), nil
				}
			}
			return 0, nil
		}
		_, err = fmt.Fprintln(stdinPipe, line)
		if err != nil {
			return 0, err
		}
		err = rl.SaveHistory(line)
		if err != nil {
			log.Warn(err)
		}
	}
}
