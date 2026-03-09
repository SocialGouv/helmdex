package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"helmdex/internal/config"
	"helmdex/internal/helmutil"

	tea "github.com/charmbracelet/bubbletea"
)

type ScreenID string

const (
	ScreenDashboard ScreenID = "dashboard"
	ScreenInstance  ScreenID = "instance"
)

type Params struct {
	RepoRoot   string
	ConfigPath string
	Config     *config.Config

	StartScreen ScreenID
}

func Run(ctx context.Context, p Params) error {
	if p.RepoRoot == "" {
		return fmt.Errorf("repoRoot is required")
	}
	if p.StartScreen == "" {
		p.StartScreen = ScreenDashboard
	}

	// Wire bundled-helm download events to the TUI status spinner.
	events := make(chan helmutil.BundledHelmEvent, 8)
	helmutil.SetBundledHelmEventSink(events)
	defer helmutil.SetBundledHelmEventSink(nil)

	m := NewAppModel(p)
	opts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithContext(ctx)}
	if strings.TrimSpace(os.Getenv("HELMDEX_MOUSE")) == "1" {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	prog := tea.NewProgram(m, opts...)

	// Forward helmutil events into the Bubble Tea update loop.
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case ev := <-events:
				switch ev.Kind {
				case helmutil.BundledHelmDownloadStart:
					prog.Send(helmDownloadStartMsg{version: ev.Version})
				case helmutil.BundledHelmDownloadDone:
					prog.Send(helmDownloadDoneMsg{version: ev.Version, err: ev.Err})
				default:
					// ignore
				}
			}
		}
	}()

	_, err := prog.Run()
	return err
}
