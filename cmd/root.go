package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"ohmywhisper/api"
	"ohmywhisper/auth"
	"ohmywhisper/config"
	"ohmywhisper/model"
)

var rootCmd = &cobra.Command{
	Use:          "ohmywhisper",
	SilenceUsage: true,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the API server",
	RunE:  runServe,
}

var (
	port      int
	modelFlag string
	authToken string
	noGPU     bool
	gpuDevice int
)

func init() {
	serveCmd.Flags().IntVarP(&port, "port", "p", 3199, "port to listen on")
	serveCmd.Flags().StringVar(&modelFlag, "model", "", "model name or path to preload")
	serveCmd.Flags().StringVar(&authToken, "token", "", "bearer token for authentication")
	serveCmd.Flags().BoolVar(&noGPU, "no-gpu", false, "disable GPU acceleration")
	serveCmd.Flags().IntVar(&gpuDevice, "gpu-device", 0, "GPU device index")
	createCmd.Flags().StringVarP(&createModelfilePath, "file", "f", "", "path to Modelfile")
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(psCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(coverCmd)
	rootCmd.AddCommand(addCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if noGPU {
		cfg.GPU = false
	}
	cfg.GPUDevice = gpuDevice

	pool := model.NewPool(cfg)
	defer pool.Close()

	if modelFlag != "" {
		name := deriveModelName(modelFlag)
		if err := pool.LoadPath(name, resolveOrPath(modelFlag, cfg)); err != nil {
			return fmt.Errorf("load model: %w", err)
		}
	}

	h := api.NewClient(pool)
	var mw []gin.HandlerFunc
	if authToken != "" {
		mw = append(mw, auth.Bearer(authToken))
	}
	return api.Serve(h, port, mw...)
}

func deriveModelName(s string) string {
	base := filepath.Base(s)
	base = strings.TrimSuffix(base, ".bin")
	base = strings.TrimPrefix(base, "ggml-")
	return base
}

func resolveOrPath(nameOrPath string, cfg *config.Config) string {
	if filepath.IsAbs(nameOrPath) {
		return nameOrPath
	}
	if p, err := model.ResolvePath(nameOrPath, cfg); err == nil {
		return p
	}
	return nameOrPath
}

var pullCmd = &cobra.Command{
	Use:   "pull <model>",
	Short: "Download a model from the hub",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		return model.Pull(args[0], cfg)
	},
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List downloaded models",
	RunE:  runList,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List downloaded models",
	RunE:  runList,
}

func runList(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	models, err := model.List(cfg)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		fmt.Println("no models found")
		fmt.Println("run 'ohmywhisper pull <model>' to download a model")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE\tMODIFIED")
	for _, m := range models {
		fmt.Fprintf(w, "%s\t%s\t%s\n", m.Name, model.HumanSize(m.Size), m.ModTime.Format("2006-01-02 15:04:05"))
	}
	w.Flush()
	return nil
}

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List running models",
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		resp, err := http.Get(cfg.ServerURL + "/api/ps")
		if err != nil {
			return fmt.Errorf("server not running at %s", cfg.ServerURL)
		}
		defer resp.Body.Close()

		var ps struct {
			Models []struct {
				Name  string `json:"name"`
				Path  string `json:"path"`
				Since string `json:"since"`
			} `json:"models"`
			RSSMB  int64   `json:"rss_mb"`
			CPUPct float64 `json:"cpu_pct"`
			GPU    *struct {
				Name   string  `json:"name"`
				Pct    float64 `json:"pct"`
				VRAMMB int64   `json:"vram_mb"`
			} `json:"gpu"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
			return err
		}

		if len(ps.Models) == 0 {
			fmt.Println("no models running")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSINCE\tPATH")
		for _, m := range ps.Models {
			fmt.Fprintf(w, "%s\t%s\t%s\n", m.Name, m.Since, m.Path)
		}
		w.Flush()

		fmt.Println()
		fmt.Printf("RAM:  %s\n", model.HumanSize(ps.RSSMB*1024*1024))
		fmt.Printf("CPU:  %.1f%%\n", ps.CPUPct)
		if ps.GPU != nil {
			fmt.Printf("GPU:  %s  %.0f%%  VRAM %s\n",
				ps.GPU.Name,
				ps.GPU.Pct,
				model.HumanSize(ps.GPU.VRAMMB*1024*1024),
			)
		}
		return nil
	},
}

var startCmd = &cobra.Command{
	Use:   "start <model>",
	Short: "Load a model into the running server",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		body, _ := json.Marshal(map[string]string{"name": args[0]})
		resp, err := http.Post(cfg.ServerURL+"/api/load", "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("server not running at %s", cfg.ServerURL)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			var e struct{ Error string `json:"error"` }
			json.NewDecoder(resp.Body).Decode(&e)
			return fmt.Errorf("%s", e.Error)
		}
		fmt.Printf("started %s\n", args[0])
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop <model>",
	Short: "Unload a model from the running server",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		req, _ := http.NewRequest(http.MethodDelete, cfg.ServerURL+"/api/unload/"+args[0], nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("server not running at %s", cfg.ServerURL)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			var e struct{ Error string `json:"error"` }
			json.NewDecoder(resp.Body).Decode(&e)
			return fmt.Errorf("%s", e.Error)
		}
		fmt.Printf("stopped %s\n", args[0])
		return nil
	},
}

var rmCmd = &cobra.Command{
	Use:   "rm <model>",
	Short: "Remove a downloaded model",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := model.Remove(args[0], cfg); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", args[0])
		return nil
	},
}

var showCmd = &cobra.Command{
	Use:   "show <model>",
	Short: "Show model details",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		m, err := model.Show(args[0], cfg)
		if err != nil {
			return err
		}
		fmt.Printf("Name:     %s\n", m.Name)
		fmt.Printf("Path:     %s\n", m.Path)
		fmt.Printf("Size:     %s\n", model.HumanSize(m.Size))
		fmt.Printf("Modified: %s\n", m.ModTime.Format(time.DateTime))
		if entry := model.FindByName(m.Name); entry != nil {
			fmt.Printf("Info:     %s (%s)\n", entry.Desc, entry.Size)
		}
		mfPath := strings.TrimSuffix(m.Path, ".bin") + ".modelfile"
		if data, err := os.ReadFile(mfPath); err == nil {
			fmt.Printf("\nModelfile:\n%s\n", string(data))
		}
		return nil
	},
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search available models in the hub catalog",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			query = args[0]
		}
		results := model.Search(query)
		if len(results) == 0 {
			fmt.Println("no models found")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSIZE\tDESCRIPTION")
		for _, e := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\n", e.Name, e.Size, e.Desc)
		}
		w.Flush()
		return nil
	},
}

var createModelfilePath string

var createCmd = &cobra.Command{
	Use:   "create <name> -f <Modelfile>",
	Short: "Create a model from a Modelfile",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if createModelfilePath == "" {
			return fmt.Errorf("-f Modelfile is required")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		return model.CreateModel(args[0], createModelfilePath, cfg)
	},
}

var coverCmd = &cobra.Command{
	Use:   "cover <src> [name]",
	Short: "Convert safetensors/PyTorch model to whisper.cpp .bin format",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		return model.Convert(args[0], name, cfg)
	},
}

var addCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Register an additional model hub URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		url := args[0]
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		for _, h := range cfg.ExtraHubs {
			if h == url {
				fmt.Printf("hub already registered: %s\n", url)
				return nil
			}
		}
		cfg.ExtraHubs = append(cfg.ExtraHubs, url)
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("added hub: %s\n", url)
		return nil
	},
}

func execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
