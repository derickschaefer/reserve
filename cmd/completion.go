package cmd

import (
	"github.com/spf13/cobra"
)

// completionCmd wraps Cobra's built-in shell completion generator.
// Running `reserve completion bash` prints a script the user can source.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for reserve.

To load completions in the current shell session:

  # bash
  source <(reserve completion bash)

  # zsh
  source <(reserve completion zsh)

  # fish
  reserve completion fish | source

Persist across sessions by adding the source line to your shell profile
(~/.bashrc, ~/.zshrc, ~/.config/fish/completions/reserve.fish, etc.).`,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactValidArgs(1),
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := cmd.Root()
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(cmd.OutOrStdout())
		case "zsh":
			return root.GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		case "powershell":
			return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		default:
			return cmd.Help()
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
