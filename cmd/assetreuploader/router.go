package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/request"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/response"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/color"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/console"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/files"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/clientpool"
)

var CompatiblePluginVersion = ""

func getOutputFileName(reuploadType string) string {
	t := time.Now()
	return fmt.Sprintf("Output_%s_%s.json", reuploadType, t.Format("2006-01-02_15-04-05"))
}

func promptSaveReuploaded(items []response.ResponseItem) {
	if len(items) == 0 {
		return
	}

	fmt.Println()
	answer, err := console.Input(fmt.Sprintf("Save the %d already-reuploaded asset id(s) to a file for a future upload? (y/n): ", len(items)))
	if err != nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
	default:
		return
	}

	data, err := json.Marshal(items)
	if err != nil {
		color.Error.Println("Failed to encode the reuploaded list: ", err)
		return
	}

	name := fmt.Sprintf("Reuploaded_%s.json", time.Now().Format("2006-01-02_15-04-05"))
	if err := files.Write(name, string(data)); err != nil {
		color.Error.Println("Failed to save the reuploaded list: ", err)
		return
	}
	color.Success.Println("Saved the already-reuploaded ids to " + name)
	fmt.Println("  Load it later with the plugin's Replace tab to swap these ids into a game.")
}

type jsonExporter struct {
	items chan response.ResponseItem
	done  chan struct{}
}

func startJSONExporter(filename string) *jsonExporter {
	e := &jsonExporter{
		items: make(chan response.ResponseItem, 256),
		done:  make(chan struct{}),
	}

	go func() {
		defer close(e.done)

		history := make([]response.ResponseItem, 0)
		for item := range e.items {
			history = append(history, item)

			for drained := false; !drained; {
				select {
				case it := <-e.items:
					history = append(history, it)
				default:
					drained = true
				}
			}

			j, err := json.Marshal(history)
			if err != nil {
				color.Error.Println("Failed to encode JSON export: ", err)
				continue
			}
			if err := files.Write(filename, string(j)); err != nil {
				color.Error.Println("Failed to write JSON export to "+filename+": ", err)
				continue
			}
		}
	}()

	return e
}

func (e *jsonExporter) close() {
	close(e.items)
	<-e.done
}

type reuploadJob struct {
	run        func() error
	exportJSON bool
	assetType  string
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

func printSummary(assetType string, p response.Progress, skipLines []string, skipUnapplied int, d time.Duration) {
	notAttempted := p.Total - p.Succeeded - p.Failed - skipUnapplied
	if notAttempted < 0 {
		notAttempted = 0
	}

	fmt.Println()
	if p.StopReason != "" {
		color.Warn.Println(fmt.Sprintf("Stopped after %s - %s", formatDuration(d), p.StopReason))
		color.Success.Println(fmt.Sprintf("  %d of %d %s assets reuploaded and will be applied", p.Succeeded, p.Total, assetType))
		if p.Failed > 0 {
			color.Error.Println(fmt.Sprintf("  %d failed (listed above)", p.Failed))
		}
		if notAttempted > 0 {
			color.Warn.Println(fmt.Sprintf("  %d not attempted", notAttempted))
		}
	} else {
		color.Success.Println(fmt.Sprintf("Done in %s - reuploaded %d out of %d %s assets", formatDuration(d), p.Succeeded, p.Total, assetType))
		if p.Failed > 0 {
			color.Error.Println(fmt.Sprintf("  %d failed (listed above)", p.Failed))
		}
	}

	for _, line := range skipLines {
		color.Warn.Println("  " + line)
	}

	fmt.Println()
	fmt.Println("Waiting for the plugin to finish changing ids...")
}

func serve(pool *clientpool.Pool) error {
	var stateMu sync.Mutex
	active := 0
	finished := true
	runErrored := false

	var exporter atomic.Pointer[jsonExporter]

	resp := response.New(func(i response.ResponseItem) {
		if e := exporter.Load(); e != nil {
			e.items <- i
		}
	})

	jobs := make(chan reuploadJob, 64)

	processJob := func(j reuploadJob) {
		var runErr error

		verbosef("Starting %s reupload job", j.assetType)
		if j.exportJSON {
			name := getOutputFileName(j.assetType)
			verbosef("Writing JSON export to %s", name)
			exporter.Store(startJSONExporter(name))
		}

		defer func() {
			rec := recover()

			if exp := exporter.Swap(nil); exp != nil {
				exp.close()
			}

			stateMu.Lock()
			active--
			if rec != nil || runErr != nil {
				runErrored = true
			}
			stateMu.Unlock()

			if rec != nil {
				color.Error.Println(fmt.Sprintf("Reupload job panicked: %v\n%s", rec, debug.Stack()))
			}
			verbosef("Finished %s reupload job", j.assetType)
		}()

		start := time.Now()
		runErr = j.run()
		if runErr != nil {
			color.Error.Println("Failed to start reuploading: ", runErr)
			return
		}
		printSummary(j.assetType, resp.Progress(), resp.SkipLines(), resp.SkipUnapplied(), time.Since(start))

		if resp.Moderated() {
			promptSaveReuploaded(resp.All())
		}
	}

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for j := range jobs {
			processJob(j)
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", localOnly(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		if resp.Len() == 0 {
			stateMu.Lock()
			if active == 0 {
				justFinished := !finished
				finished = true
				var errored bool
				if justFinished {
					errored = runErrored
					runErrored = false
				}
				stateMu.Unlock()

				if justFinished {
					fmt.Fprint(w, "done")
					if errored {
						color.Error.Println("Reupload failed. Ready to try again.")
					} else {
						color.Success.Println("All ids replaced. Ready for another reupload.")
					}
				}
				return
			}
			stateMu.Unlock()
		}

		if err := resp.EncodeJSON(json.NewEncoder(w)); err != nil {
			color.Error.Println("Failed to encode response: ", err)
		} else {
			resp.Clear()
		}
	}))

	mux.HandleFunc("GET /progress", localOnly(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp.Progress()); err != nil {
			color.Error.Println("Failed to encode progress: ", err)
		}
	}))

	mux.HandleFunc("POST /reupload", localOnly(func(w http.ResponseWriter, r *http.Request) {
		var req request.RawRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			color.Error.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if CompatiblePluginVersion != "" && req.PluginVersion != CompatiblePluginVersion {
			verbosef("Rejected reupload: plugin version %q does not match required %q", req.PluginVersion, CompatiblePluginVersion)
			w.WriteHeader(http.StatusConflict)
			return
		}

		if exists := assets.DoesModuleExist(req.AssetType); !exists {
			verbosef("Rejected reupload: unknown asset type %q", req.AssetType)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		startReupload, err := assets.NewReuploadHandlerWithType(req.AssetType, pool, &req, resp)
		if err != nil {
			color.Error.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		stateMu.Lock()
		active++
		finished = false
		stateMu.Unlock()

		verbosef("Queued %s reupload: %d id(s), creatorId=%d, group=%t, exportJSON=%t", req.AssetType, len(req.IDs), req.CreatorID, req.IsGroup, req.ExportJSON)
		jobs <- reuploadJob{run: startReupload, exportJSON: req.ExportJSON, assetType: req.AssetType}
		w.WriteHeader(http.StatusOK)
	}))

	srv := &http.Server{
		Addr:              "127.0.0.1:" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownResult := make(chan error, 1)
	go func() {
		<-ctx.Done()
		verboseln("Received shutdown signal")
		fmt.Println()
		fmt.Println("Shutting down...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownResult <- srv.Shutdown(shutdownCtx)
	}()

	verbosef("HTTP server starting on %s", srv.Addr)
	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	if shutdownErr := <-shutdownResult; shutdownErr == nil {
		close(jobs)
		<-workerDone
	}
	return nil
}

func localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hostIsLoopback(r.Host) {
			verbosef("Rejected %s %s: non-loopback host %q", r.Method, r.URL.Path, r.Host)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !originIsLoopback(origin) {
			verbosef("Rejected %s %s: non-loopback origin %q", r.Method, r.URL.Path, origin)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func hostIsLoopback(host string) bool {
	if host == "" {
		return false
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	return isLoopbackHostname(hostname)
}

func originIsLoopback(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isLoopbackHostname(u.Hostname())
}

func isLoopbackHostname(h string) bool {
	h = strings.Trim(strings.TrimSpace(strings.ToLower(h)), "[]")
	if h == "localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
