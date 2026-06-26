package main

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"ohmywhisper/api"
	whisperlib "ohmywhisper/api/whisper"
	"ohmywhisper/auth"
)

var rootCmd = &cobra.Command{
	Use:          "ohmywhisper",
	SilenceUsage: true,
}

var serveCmd = &cobra.Command{
	Use:  "serve",
	RunE: runServe,
}

var (
	port      int
	modelPath string
	authToken string
)

func init() {
	serveCmd.Flags().IntVar(&port, "port", 3199, "port to listen on")
	serveCmd.Flags().StringVar(&modelPath, "model", "", "path to whisper ggml model file")
	serveCmd.Flags().StringVar(&authToken, "token", "", "bearer token for authentication (optional)")
	_ = serveCmd.MarkFlagRequired("model")
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	t, err := whisperlib.NewEngine(modelPath)
	if err != nil {
		return fmt.Errorf("load model: %w", err)
	}
	defer t.Close()

	h := api.NewClient(t)

	var mw []gin.HandlerFunc
	if authToken != "" {
		mw = append(mw, auth.Bearer(authToken))
	}

	return api.Serve(h, port, mw...)
}

func execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
