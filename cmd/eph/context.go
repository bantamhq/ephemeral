package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"ephemeral/internal/config"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage authentication contexts",
		Long:  `List and switch between authentication contexts.`,
		RunE:  runContextList,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "use <name>",
			Short: "Switch to a different context",
			Args:  cobra.ExactArgs(1),
			RunE:  runContextUse,
		},
	)

	return cmd
}

func runContextList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil || len(cfg.Contexts) == 0 {
		fmt.Println("No contexts configured. Run 'eph login' to authenticate.")
		return nil
	}

	var names []string
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		ctx := cfg.Contexts[name]
		marker := "  "
		if name == cfg.CurrentContext {
			marker = "* "
		}
		fmt.Printf("%s%s (%s)\n", marker, name, ctx.Server)
	}

	return nil
}

func runContextUse(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("context %q not found", name)
	}

	cfg.CurrentContext = name

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Switched to context %q\n", name)
	return nil
}
