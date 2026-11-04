package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"

	"wordhoard/internal/db"
	"wordhoard/internal/enrich"
	"wordhoard/internal/ingest"
	"wordhoard/internal/server"
	"wordhoard/internal/translate"
)

// newTranslator builds the configured online Translator; tests replace it with a fake.
var newTranslator = func() (translate.Translator, error) { return translate.NewFromEnv() }

func main() {
	if err := runCLI(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: wordhoard <ingest|enrich|serve> [options]")
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

	case "enrich":
		return runEnrich(args[1:], out)

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

		tr, err := newTranslator()
		if err != nil {
			// Serve anyway; "Fetch translations" reports the configuration error.
			fmt.Fprintf(out, "warning: online translator unavailable: %v\n", err)
			tr = translate.Unavailable(err)
		}

		handler := server.NewHandler(store, spaFS, server.WithTranslator(tr))
		fmt.Fprintf(out, "Listening on %s (database: %s)\n", *addr, *dbPath)
		return http.ListenAndServe(*addr, handler)

	default:
		return fmt.Errorf("unknown subcommand %q (expected 'ingest', 'enrich' or 'serve')", subcmd)
	}
}

func runEnrich(args []string, out io.Writer) error {
	fsFlags := flag.NewFlagSet("enrich", flag.ContinueOnError)
	fsFlags.SetOutput(out)
	dbPath := fsFlags.String("db", "wordhoard.db", "path to SQLite database")
	bookID := fsFlags.Int64("book", 0, "only enrich lexemes from this book id (default: all books)")
	limit := fsFlags.Int("limit", 0, "maximum number of lexemes to enrich (default: no limit)")
	batchSize := fsFlags.Int("batch-size", enrich.DefaultBatchSize, "lexemes per translation request")
	noTUI := fsFlags.Bool("no-tui", false, "print plain progress lines instead of the live progress bar")
	if err := fsFlags.Parse(args); err != nil {
		return err
	}
	if *batchSize <= 0 {
		return errors.New("-batch-size must be positive")
	}
	if *limit < 0 || *bookID < 0 {
		return errors.New("-limit and -book must not be negative")
	}

	tr, err := newTranslator()
	if err != nil {
		return err
	}

	store, err := db.Open(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	summary, err := enrich.RunTUI(context.Background(), store, tr, enrich.Options{
		BookID:    *bookID,
		Limit:     *limit,
		BatchSize: *batchSize,
	}, out, *noTUI)
	if err != nil {
		return err
	}
	if summary.Failed > 0 {
		return fmt.Errorf("%d of %d lexemes failed to translate", summary.Failed, summary.Total)
	}
	return nil
}
