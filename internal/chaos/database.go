package chaos

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/thor/dumbs/internal/logger"
)

// runDatabase launches three sub-goroutines that abuse the database in
// different instructive ways. All three run under the same context; cancelling
// it terminates all of them.
//
//   - rapidInserts: batch-inserts 1 000 rows as fast as possible — fills the
//     table and spikes CPU/IO.
//   - hangingTxns: opens transactions and then sleeps forever without committing
//     — exhausts connection slots; visible in pg_stat_activity as
//     "idle in transaction".
//   - badQueries: sends queries referencing non-existent columns/tables —
//     floods logs with ERROR lines; teaches log triage.
func (m *Manager) runDatabase(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); m.rapidInserts(ctx) }()
	go func() { defer wg.Done(); m.hangingTxns(ctx) }()
	go func() { defer wg.Done(); m.badQueries(ctx) }()
	wg.Wait()
}

// rapidInserts inserts 1 000-row batches into events as quickly as possible.
func (m *Manager) rapidInserts(ctx context.Context) {
	pool := m.db.Pool()
	for ctx.Err() == nil {
		rows := make([][]any, 1000)
		for i := range rows {
			rows[i] = []any{randomPayload(64)}
		}
		_, err := pool.CopyFrom(
			ctx,
			[]string{"events"},
			[]string{"payload"},
			newRowSrc(rows),
		)
		if err != nil && ctx.Err() == nil {
			logger.Get().Error().Err(err).Msg("chaos/database: rapid insert failed")
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// hangingTxns opens transactions, runs a SELECT, then sleeps forever without
// committing. Visible as "idle in transaction" in pg_stat_activity.
// The lesson: idle_in_transaction_session_timeout and connection-pool exhaustion.
func (m *Manager) hangingTxns(ctx context.Context) {
	pool := m.db.Pool()
	var txMu sync.Mutex
	var hanging []context.CancelFunc

	defer func() {
		txMu.Lock()
		for _, c := range hanging {
			c()
		}
		txMu.Unlock()
	}()

	for ctx.Err() == nil {
		// Spawn a new hanging transaction every 2 seconds.
		time.Sleep(2 * time.Second)

		txCtx, txCancel := context.WithCancel(context.Background())
		go func() {
			tx, err := pool.Begin(txCtx)
			if err != nil {
				txCancel()
				return
			}
			// Do a harmless read so the transaction is real.
			_, _ = tx.Exec(txCtx, "SELECT pg_sleep(0)")
			logger.Get().Warn().Msg("chaos/database: hanging transaction opened; will never commit")
			// Block until the worker is stopped.
			<-txCtx.Done()
			_ = tx.Rollback(context.Background())
		}()

		txMu.Lock()
		hanging = append(hanging, txCancel)
		txMu.Unlock()
	}
}

// badQueries runs SQL that references non-existent identifiers, producing loud
// errors. The lesson: ERROR log triaging, and why you don't expose raw DB errors
// to end users.
func (m *Manager) badQueries(ctx context.Context) {
	pool := m.db.Pool()
	badSQL := []string{
		"SELECT nonexistent_column FROM events",
		"SELECT * FROM table_that_does_not_exist",
		"INSERT INTO events (nonexistent_field) VALUES ('oops')",
		"UPDATE events SET ghost_column = 'boo' WHERE id = 1",
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := pool.Exec(ctx, badSQL[i%len(badSQL)])
			if err != nil && ctx.Err() == nil {
				logger.Get().Error().
					Err(err).
					Str("sql", badSQL[i%len(badSQL)]).
					Msg("chaos/database: intentional bad query")
			}
			i++
		}
	}
}

// randomPayload returns a hex-encoded random string of approximately n bytes.
func randomPayload(n int) string {
	return randomHex((n + 1) / 2)
}

// shuffleString returns a random alphanumeric string of length n.
func shuffleString(n int) string {
	const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alpha[rand.IntN(len(alpha))]
	}
	return string(b)
}

// rowSrc adapts [][]any to pgx's CopyFromSource interface.
type rowSrc struct {
	rows [][]any
	idx  int
}

func newRowSrc(rows [][]any) *rowSrc     { return &rowSrc{rows: rows} }
func (r *rowSrc) Next() bool             { r.idx++; return r.idx <= len(r.rows) }
func (r *rowSrc) Values() ([]any, error) { return r.rows[r.idx-1], nil }
func (r *rowSrc) Err() error             { return nil }
