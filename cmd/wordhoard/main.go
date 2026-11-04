package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"

	"wordhoard/internal/db"
	"wordhoard/internal/ingest"
	"wordhoard/internal/server"
	"wordhoard/internal/translate"
)

func main() {
	if err := runCLI(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: wordhoard <ingest|serve> [options]")
	}

	subcmd := args[0]
	switch subcmd {
	case "ingest":
		fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
		fs.SetOutput(out)
		dbPath := fs.String("db", "wordhoard.db", "path to SQLite database")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		patterns := fs.Args()
		if len(patterns) == 0 {
			return errors.New("usage: wordhoard ingest [-db path] <pattern...>")
		}
		store, err := db.Open(*dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		_, err = ingest.RunTUI(store, patterns, out)
		return err

	case "serve":
		fsFlags := flag.NewFlagSet("serve", flag.ContinueOnError)
		fsFlags.SetOutput(out)
		dbPath := fsFlags.String("db", "wordhoard.db", "path to SQLite database")
		addr := fsFlags.String("addr", ":8080", "HTTP listen address")
		webDir := fsFlags.String("web", "web/dist", "path to built frontend assets")
		if err := fsFlags.Parse(args[1:]); err != nil {
			return err
		}

		store, err := db.Open(*dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		var spaFS fs.FS
		if info, err := os.Stat(*webDir); err == nil && info.IsDir() {
			spaFS = os.DirFS(*webDir)
		}

		var opts []server.Option
		if tr, err := translate.NewFromEnv(); err == nil {
			opts = append(opts, server.WithTranslator(tr))
		}

		handler := server.NewHandler(store, spaFS, opts...)
		fmt.Fprintf(out, "Listening on %s (database: %s)\n", *addr, *dbPath)
		return http.ListenAndServe(*addr, handler)

	default:
		return fmt.Errorf("unknown subcommand %q (expected 'ingest' or 'serve')", subcmd)
	}
}
