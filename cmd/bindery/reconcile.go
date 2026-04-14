package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
)

// runReconcileSeries handles the `bindery reconcile-series` subcommand.
// It iterates every author already in the library, re-fetches their works from
// OpenLibrary, and for any work that already exists as a book row it upserts
// the series + series_books data. Run this once after upgrading from a version
// that did not populate series during ingestion.
func runReconcileSeries(
	authorRepo *db.AuthorRepo,
	bookRepo *db.BookRepo,
	seriesRepo *db.SeriesRepo,
	agg *metadata.Aggregator,
) {
	ctx := context.Background()

	authors, err := authorRepo.List(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "list authors:", err)
		os.Exit(1)
	}

	var linked, skipped int
	for _, author := range authors {
		works, err := agg.GetAuthorWorks(ctx, author.ForeignID)
		if err != nil {
			slog.Warn("reconcile-series: failed to fetch works", "author", author.Name, "error", err)
			continue
		}

		for _, w := range works {
			if len(w.SeriesRefs) == 0 {
				continue
			}
			existing, err := bookRepo.GetByForeignID(ctx, w.ForeignID)
			if err != nil || existing == nil {
				skipped++
				continue
			}
			for _, ref := range w.SeriesRefs {
				s := &models.Series{ForeignID: ref.ForeignID, Title: ref.Title}
				if err := seriesRepo.CreateOrGet(ctx, s); err != nil {
					slog.Warn("reconcile-series: upsert series failed", "series", ref.Title, "error", err)
					continue
				}
				if err := seriesRepo.LinkBook(ctx, s.ID, existing.ID, ref.Position, ref.Primary); err != nil {
					slog.Warn("reconcile-series: link failed", "book", existing.Title, "series", ref.Title, "error", err)
					continue
				}
				linked++
			}
		}
		slog.Info("reconcile-series: processed author", "author", author.Name)
	}

	fmt.Printf("{\"linked\":%d,\"skipped\":%d}\n", linked, skipped)
}
