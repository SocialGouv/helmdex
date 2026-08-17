package cli

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/repo"
	"helmdex/internal/server"

	"github.com/spf13/cobra"
)

func newUICmd(f *rootFlags) *cobra.Command {
	var port int
	var host string
	var open bool

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Serve the web UI (local HTTP server + API)",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			res, err := config.Resolve(repoRoot, f.Config)
			if err != nil {
				return err
			}
			cfg := instances.ApplyLayout(repoRoot, res)

			srv := server.New(server.Params{
				RepoRoot: repoRoot,
				Config:   cfg,
				Resolved: res,
			})
			stopEvents := srv.StartHelmEventForwarding()
			defer stopEvents()

			addr := fmt.Sprintf("%s:%d", host, port)
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", addr, err)
			}
			url := fmt.Sprintf("http://%s", ln.Addr().String())
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "helmdex ui serving %s at %s\n", repoRoot, url)

			if open {
				go func() {
					time.Sleep(200 * time.Millisecond)
					_ = openBrowser(url)
				}()
			}

			httpSrv := &http.Server{
				Handler:           srv.Handler(),
				ReadHeaderTimeout: 10 * time.Second,
			}
			go func() {
				<-cmd.Context().Done()
				_ = httpSrv.Close()
			}()
			if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&port, "port", 8117, "Port to listen on (0 = random free port)")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host to bind (localhost only by default)")
	cmd.Flags().BoolVar(&open, "open", false, "Open the UI in the default browser")
	return cmd
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
