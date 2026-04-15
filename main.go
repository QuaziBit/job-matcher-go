package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/QuaziBit/job-matcher-go/assets"
	"github.com/QuaziBit/job-matcher-go/config"
	"github.com/QuaziBit/job-matcher-go/launcher"
	"github.com/QuaziBit/job-matcher-go/server"
)

func main() {
	// ── Load config ──────────────────────────────────────────────────────────
	cfgPath := config.ConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config from %s: %v", cfgPath, err)
	}

	printBanner(cfgPath)

	// ── Start launcher ───────────────────────────────────────────────────────
	startCh := make(chan config.Config, 1)
	l := launcher.New(cfg, cfgPath, startCh)

	launcherURL, err := l.Start()
	if err != nil {
		log.Fatalf("Failed to start launcher: %v", err)
	}

	fmt.Printf("  Launcher   %s\n\n", launcherURL)
	fmt.Println("  Opening launcher in your browser…")
	fmt.Println("  Configure settings and click Start.")
	fmt.Println()
	openBrowser(launcherURL)

	// ── Wait for user to click Start ─────────────────────────────────────────
	appCfg := <-startCh

	keyStatus := "not set"
	if appCfg.AnthropicAPIKey != "" && appCfg.AnthropicAPIKey != "sk-ant-..." {
		masked := appCfg.AnthropicAPIKey[:min(12, len(appCfg.AnthropicAPIKey))] + "..." +
			appCfg.AnthropicAPIKey[len(appCfg.AnthropicAPIKey)-4:]
		keyStatus = masked
	}
	fmt.Printf("\n  Config loaded:\n")
	fmt.Printf("    Anthropic key : %s\n", keyStatus)
	fmt.Printf("    Ollama model  : %s\n", appCfg.OllamaModel)
	fmt.Printf("    DB path       : %s\n", appCfg.DBPath)
	fmt.Printf("\n  Starting app on http://%s:%d …\n\n", appCfg.Host, appCfg.Port)

	// ── Start main app server ────────────────────────────────────────────────
	appServer := server.New(appCfg, assets.UI)
	if err := appServer.Start(); err != nil {
		log.Fatalf("Failed to start app server: %v", err)
	}
	fmt.Printf("  ✓  Job Matcher running at http://%s:%d\n", appCfg.Host, appCfg.Port)
	fmt.Println("  Press Ctrl+C to stop.")
	fmt.Println()

	// ── Event loop — handle stop, restart, OS signals ─────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-quit:
			fmt.Println("\n  Shutting down…")
			appServer.Stop()
			l.Stop()
			fmt.Println("  Stopped. Goodbye!")
			return

		case <-l.StopCh():
			log.Printf("→ Stop signal received from launcher")
			appServer.Stop()
			fmt.Println("\n  ✓  App server stopped.")
			fmt.Println("  Launcher still open — click Start to run again.")
			// Wait for next start or OS signal
			select {
			case newCfg := <-startCh:
				log.Printf("→ Restart after stop: port=%d model=%s", newCfg.Port, newCfg.OllamaModel)
				appServer = server.New(newCfg, assets.UI)
				if err := appServer.Start(); err != nil {
					log.Printf("✗ Failed to restart app server: %v", err)
				} else {
					fmt.Printf("\n  ✓  Job Matcher running at http://%s:%d\n", newCfg.Host, newCfg.Port)
				}
			case <-quit:
				fmt.Println("\n  Shutting down…")
				l.Stop()
				fmt.Println("  Stopped. Goodbye!")
				return
			}

		case newCfg := <-l.RestartCh():
			log.Printf("→ Restart signal received: port=%d model=%s", newCfg.Port, newCfg.OllamaModel)
			fmt.Printf("\n  ↺  Restarting on http://%s:%d …\n", newCfg.Host, newCfg.Port)
			appServer.Stop()
			appServer = server.New(newCfg, assets.UI)
			if err := appServer.Start(); err != nil {
				log.Printf("✗ Failed to restart app server: %v", err)
			} else {
				fmt.Printf("  ✓  Job Matcher running at http://%s:%d\n\n", newCfg.Host, newCfg.Port)
			}
		}
	}
}

func printBanner(cfgPath string) {
	cyan := "\033[96m"
	bold := "\033[1m"
	reset := "\033[0m"

	fmt.Printf("\n%s%s%s\n", cyan, "══════════════════════════════════════════════", reset)
	fmt.Printf("%s%s   Job Matcher%s\n", cyan, bold, reset)
	fmt.Printf("%s%s\n\n", cyan, "══════════════════════════════════════════════"+reset)
	fmt.Printf("  Config     %s\n", cfgPath)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Could not open browser automatically: %v", err)
		fmt.Printf("  Please open manually: %s\n", url)
	}
}
