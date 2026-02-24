package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "airadar",
		Short: "Detect trending AI products and news from multiple sources",
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./config.yaml)")

	root.AddCommand(collectCmd())
	root.AddCommand(trendsCmd())
	root.AddCommand(serveCmd())
	root.AddCommand(runCmd())

	return root
}

func collectCmd() *cobra.Command {
	var sources []string

	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Run data collectors",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollect(sources)
		},
	}

	cmd.Flags().StringSliceVar(&sources, "source", nil, "specific sources to collect (e.g., hn,github,rss)")
	return cmd
}

func trendsCmd() *cobra.Command {
	var (
		jsonOutput bool
		minScore   float64
		limit      int
	)

	cmd := &cobra.Command{
		Use:   "trends",
		Short: "Show current trending topics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrends(jsonOutput, minScore, limit)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	cmd.Flags().Float64Var(&minScore, "min-score", -1, "minimum trend score (default: from config)")
	cmd.Flags().IntVar(&limit, "limit", 20, "max trends to show")
	return cmd
}

func serveCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start HTTP API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(port)
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "server port")
	return cmd
}

func runCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start daemon with scheduler and HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(port)
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "server port")
	return cmd
}
