package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

func runCommand(args []string, cfg GrowlYaml, c *cli.Context) {
	var command GrowlCommand
	for _, cmd := range cfg.Commands {
		if cmd.Name == args[0] {
			command = cmd
			break
		}
	}
	if command.Name == "" {
		printList(cfg.Commands)
		printErr("Command not found")
	}

	for _, env := range cfg.GlobalEnv {
		os.Setenv(env.Name, env.Value)
	}
	for _, env := range command.Env {
		os.Setenv(env.Name, env.Value)
	}

	shell := cfg.Shell
	if shell == "" {
		switch runtime.GOOS {
		case "windows":
			shell = "cmd /C"
		case "linux", "darwin":
			shell = "bash -c"
		}
	}

	shellArgs := strings.Split(shell, " ")

	cmds := append([]string{command.Command}, command.Extra...)
	for _, cmd := range cmds {
		for i, arg := range args[1:] {
			cmd = strings.ReplaceAll(cmd, fmt.Sprintf("%%%d", i+1), arg)
		}

		missing := []string{}
		for _, word := range strings.Split(cmd, " ") {
			if strings.HasPrefix(word, "%") && isNumber(word[1:]) {
				missing = append(missing, word)
			}
		}
		if len(missing) > 0 {
			printErr("Missing argument(s):", strings.Join(missing, ", "))
		}

		runCmd := exec.Command(shellArgs[0])
		runCmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: shellArgs[1] + fmt.Sprintf(` "%s"`, cmd)}
		runCmd.Stderr = os.Stderr
		runCmd.Stdout = os.Stdout
		runCmd.Stdin = os.Stdin
		if err := runCmd.Run(); err != nil {
			printErr(err.Error())
		}
	}
}

func crossCompile(c *cli.Context) error {
	color.Green("Flags:")
	blue := color.New(color.FgBlue)
	blue.Print("- os ")
	os.Setenv("GOOS", c.String("os"))
	fmt.Println(c.String("os"))
	blue.Print("- arch ")
	os.Setenv("GOARCH", c.String("arch"))
	fmt.Println(c.String("arch"))
	ld := c.String("ldflags")
	if c.Bool("static") {
		ld += " -extldflags=-static"
	}
	if c.Bool("light") {
		ld += " -w -s"
	}
	if c.Bool("noconsole") {
		ld += " -H=windowsgui"
	}
	ld = strings.Trim(ld, " ")

	blue.Print("- ldflags ")
	fmt.Println(ld)
	blue.Print("- cgo ")
	fmt.Println(c.Bool("cgo"))
	if c.Bool("cgo") {
		os.Setenv("CGO_ENABLED", "1")
	}
	blue.Print("- out ")
	out := c.String("out")
	if out == "" {
		out = "bin/" + os.Getenv("GOOS") + "-" + os.Getenv("GOARCH")
	}
	if os.Getenv("GOOS") == "windows" {
		out += ".exe"
	}
	fmt.Println(out)
	color.Green("Building...")
	args := append([]string{
		"build", "-ldflags=" + ld, "-o=" + out},
		c.Args().Slice()...,
	)
	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		// check if file exists
		if _, err := os.Stat("growl.yaml"); err == nil {
			printErr("Growl.yaml already exists")
		}
		initYaml()
		return
	}
	var cfg GrowlYaml
	content, err := os.ReadFile("growl.yaml")
	if err != nil {
		printErr("Growl.yaml not found. Generate one with growl init")
	}
	yaml.Unmarshal(content, &cfg)
	app := cli.App{
		Name:                 "growl",
		Usage:                "simple go cli tools",
		EnableBashCompletion: true,
		SkipFlagParsing:      true,
		Action: func(c *cli.Context) error {
			if len(c.Args().Slice()) == 0 {
				cmd := exec.Command("go", "run", ".")
				cmd.Stderr = os.Stderr
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				if err := cmd.Run(); err != nil {
					printErr(err.Error())
				}
			} else {
				runCommand(c.Args().Slice(), cfg, c)
			}
			return nil
		},
		Commands: []*cli.Command{
			{
				Name: "list",
				Action: func(c *cli.Context) error {
					printList(cfg.Commands)
					return nil
				},
				Usage: "List commands from growl.yaml",
				Aliases: []string{
					"l",
				},
			},
			{
				Name: "help",
				Action: func(c *cli.Context) error {
					if c.Args().Len() > 0 {
						for _, v := range c.App.Commands {
							if v.Name == c.Args().Get(0) {
								fmt.Println(v.UsageText)
							}
						}
						return nil
					}
					cli.ShowAppHelp(c)
					return nil
				},
				Usage: "Shows this help",
				Aliases: []string{
					"h",
				},
			},
			{
				Name: "cross",
				UsageText: "growl cross -os [os] -arch [arch] -ldflags \"[ldflags]\" [-static] [-light] [-cgo] -out [output] -noconsole\n" +
					"growl cross -os [os] -a [arch] -ld \"[ldflags]\" [-s] [-l] [-c] -o [output] -nc\n" +
					"Default output is bin/$GOOS-$GOARCH\n" +
					"--noconsole (or -nc) disables the console in windows to use only the GUI (adds -H=windowsgui ldflag). \n" +
					"You can use growl cross list to list available OS and CPU architectures",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "os",
						Aliases: []string{},
						Value:   runtime.GOOS,
					},
					&cli.StringFlag{
						Name:    "arch",
						Aliases: []string{"a"},
						Value:   runtime.GOARCH,
					},
					&cli.StringFlag{
						Name:    "ldflags",
						Aliases: []string{"ld"},
						Value:   "",
					},
					&cli.StringFlag{
						Name:    "out",
						Aliases: []string{"o"},
						Value:   "",
					},
					&cli.BoolFlag{
						Name:    "static",
						Aliases: []string{"s"},
						Value:   false,
					},
					&cli.BoolFlag{
						Name:    "noconsole",
						Aliases: []string{"nc"},
						Value:   false,
					},
					&cli.BoolFlag{
						Name:    "light",
						Aliases: []string{"l"},
						Value:   false,
					},
					&cli.BoolFlag{
						Name:    "cgo",
						Aliases: []string{"c"},
						Value:   os.Getenv("CGO_ENABLED") == "1",
					},
				},
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List available OS and CPU architectures",
						Action: func(c *cli.Context) error {
							color.Green("Available OS:")
							for _, v := range knownOS {
								fmt.Println(v)
							}
							color.Green("Available CPU architectures:")
							for _, v := range knownArch {
								fmt.Println(v)
							}
							return nil
						},
					},
				},
				Action: func(c *cli.Context) error {
					return crossCompile(c)
				},
				Usage: "Build to target OS and arch. (build normally if not specified) (growl help cross for more info)",
				Aliases: []string{
					"c",
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		printErr(err.Error())
	}
}
