package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := newRoot().ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "Failed: canceled")
		} else {
			fmt.Fprintln(os.Stderr, "Failed:", err)
		}
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	var configPath, output string
	root := &cobra.Command{Use: "go-trans", Short: "Subtitles for videos.", Version: version, SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath(), "Config file")
	generate := &cobra.Command{Use: "generate VIDEO", Short: "Generate and embed subtitles.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig(configPath)
		if err != nil {
			return err
		}
		return generateVideo(cmd.Context(), c, args[0], output)
	}}
	remove := &cobra.Command{Use: "remove VIDEO", Short: "Remove embedded subtitle tracks.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return removeVideo(cmd.Context(), args[0], output)
	}}
	for _, cmd := range []*cobra.Command{generate, remove} {
		cmd.Flags().StringVarP(&output, "output", "o", "", "Write a new video instead of replacing the input")
		cmd.ValidArgsFunction = func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"mkv", "mp4", "mov", "m4v"}, cobra.ShellCompDirectiveFilterFileExt
		}
		_ = cmd.MarkFlagFilename("output", "mkv", "mp4", "mov", "m4v")
	}
	config := &cobra.Command{Use: "config", Short: "Manage configuration."}
	config.AddCommand(&cobra.Command{Use: "init", Short: "Create a config file.", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.WriteString(configTemplate)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Println("Done:", configPath)
		return nil
	}})
	config.AddCommand(&cobra.Command{Use: "path", Short: "Print the config path.", Args: cobra.NoArgs, Run: func(_ *cobra.Command, _ []string) { fmt.Println(configPath) }})
	root.AddCommand(generate, remove, config)
	return root
}
