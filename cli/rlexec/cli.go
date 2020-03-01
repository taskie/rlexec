package rlexec

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/chzyer/readline"
	"go.uber.org/zap"

	"github.com/spf13/cobra"
	"github.com/taskie/fwv"
	"github.com/taskie/ose"
	"github.com/taskie/ose/coli"
)

const CommandName = "rlexec"

var Command *cobra.Command

func init() {
	Command = NewCommand(coli.NewColiInThisWorld())
}

func Main() {
	Command.Execute()
}

func NewCommand(cl *coli.Coli) *cobra.Command {
	cmd := &cobra.Command{
		Use:  CommandName,
		Args: cobra.ArbitraryArgs,
		Run:  cl.WrapRun(run),
	}
	cl.Prepare(cmd)

	flg := cmd.Flags()
	flg.SetInterspersed(false)
	flg.StringP("output", "o", "", "output file")
	flg.StringP("history", "H", "", "history file")
	flg.StringP("prompt", "p", "> ", "prompt")
	flg.StringP("buffered", "r", "", "use buffer for output")
	flg.StringP("temp", "t", "", "use tempfile for output")

	cl.BindFlags(flg, []string{"output", "history", "prompt", "buffered", "temp"})
	return cmd
}

type Config struct {
	Output, History, Prompt, LogLevel string
	Buffered, Temp                    bool
}

var globalExitStatus int

func run(cl *coli.Coli, cmd *cobra.Command, args []string) {
	v := cl.Viper()
	log := zap.L()
	if v.GetBool("version") {
		cmd.Println(fwv.Version)
		return
	}
	var config Config
	err := v.Unmarshal(&config)
	if err != nil {
		log.Fatal("can't unmarshal config", zap.Error(err))
	}

	output := config.Output

	opener := ose.NewOpenerInThisWorld()
	opener.Unbuffered = !config.Buffered

	if config.Temp {
		ok, err := opener.CreateTempFile("", CommandName, output, func(f io.WriteCloser) (bool, error) {
			process(f, args, &config)
			return true, nil
		})
		if !ok && err != nil {
			log.Fatal("can't create file", zap.Error(err))
		}
	} else {
		f, err := opener.Create(output)
		if err != nil {
			log.Fatal("can't create file", zap.Error(err))
		}
		defer f.Close()
		process(f, args, &config)
	}
	os.Exit(globalExitStatus)
}

func process(w io.Writer, args []string, config *Config) {
	log := zap.L()
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
			log.Error("execution failed", zap.Error(err))
		}
	} else {
		err = print(w, rl)
		if err != nil {
			log.Error("print failed", zap.Error(err))
		}
	}
}

func print(w io.Writer, rl *readline.Instance) error {
	log := zap.L()
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
			log.Warn("can't save history", zap.Error(err))
		}
	}
}

func execute(ctx context.Context, w io.Writer, rl *readline.Instance, name string, args []string) (int, error) {
	log := zap.L()
	log.Debug("executing", zap.String("name", name), zap.Strings("args", args))
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
				log.Info("can't read", zap.Error(err))
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
			log.Warn("can't save history", zap.Error(err))
		}
	}
}
