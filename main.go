// Command stock-tracker is a CLI trend monitor for a list of stocks/ETFs.
// For each tracked symbol it fetches 5-day hourly data from Yahoo Finance,
// computes trend/volume/signal metrics and position P&L, detects changes vs the
// previous run, and prints a compact report. Designed to run on a fixed
// interval by cron.
//
// Optional: if OPENAI_API_KEY is set, a cached AI "why it moved" news line is
// added on meaningful moves (see the AI_EXPLAIN_* / AI_CACHE_* env vars).
package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	// Embed the timezone database so America/New_York resolves even on systems
	// without system tzdata (keeps market-session math correct year-round).
	_ "time/tzdata"

	"stocktracker/internal/ai"
	"stocktracker/internal/config"
	"stocktracker/internal/monitor"
	"stocktracker/internal/positions"
	"stocktracker/internal/stock"
	"stocktracker/internal/store"
)

const usage = `stock-tracker — CLI stock trend monitor (Yahoo Finance)

Usage:
  stock-tracker track [--loop] [--interval SECONDS]
                              Run one monitoring pass (the cron entry point).
                              With --loop, run continuously (default 1800s).
  stock-tracker list          Show the tracked symbols.
  stock-tracker add SYM...    Add symbol(s) to the tracker list.
  stock-tracker remove SYM... Remove symbol(s) from the tracker list.
  stock-tracker position set SYM QTY AVG
                              Record/update a held position (also tracks SYM).
  stock-tracker position list Show recorded positions.
  stock-tracker position remove SYM
                              Delete a recorded position.
  stock-tracker price SYM...  Print the current price for symbol(s).
  stock-tracker research SYM...
                              Force a one-off AI news summary for symbol(s).
  stock-tracker help          Show this help.
`

func main() {
	config.LoadDotenv(".env")

	if err := store.Ensure(); err != nil {
		fatal(err)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(1)
	}

	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "track":
		err = cmdTrack(rest)
	case "list":
		err = cmdList()
	case "add":
		err = cmdAdd(rest)
	case "remove":
		err = cmdRemove(rest)
	case "position", "positions":
		err = cmdPosition(rest)
	case "price":
		err = cmdPrice(rest)
	case "research":
		err = cmdResearch(rest)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Printf("unknown command: %s\n\n%s", cmd, usage)
		os.Exit(1)
	}

	if err != nil {
		fatal(err)
	}
}

func cmdTrack(args []string) error {
	loop := false
	interval := 1800
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--loop":
			loop = true
		case "--interval":
			if i+1 >= len(args) {
				return fmt.Errorf("--interval requires a value in seconds")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid --interval: %s", args[i+1])
			}
			interval = n
			i++
		default:
			return fmt.Errorf("unknown flag for track: %s", args[i])
		}
	}

	if !loop {
		anyError, err := monitor.Run()
		if err != nil {
			return err
		}
		if anyError {
			os.Exit(1) // let cron error-alerting fire
		}
		return nil
	}

	fmt.Printf("Starting continuous monitoring every %ds. Ctrl+C to stop.\n", interval)
	for {
		if _, err := monitor.Run(); err != nil {
			fmt.Printf("monitor error: %v\n", err)
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func cmdList() error {
	symbols, err := store.LoadTrackers()
	if err != nil {
		return err
	}
	if len(symbols) == 0 {
		fmt.Println("Tracker list is empty. Add one with: stock-tracker add AAPL")
		return nil
	}
	fmt.Println("Tracking:")
	for _, s := range symbols {
		fmt.Printf("  - %s\n", s)
	}
	return nil
}

func cmdAdd(symbols []string) error {
	if len(symbols) == 0 {
		return fmt.Errorf("add requires at least one symbol")
	}
	for _, s := range symbols {
		added, err := store.AddTracker(s)
		if err != nil {
			return err
		}
		if added {
			fmt.Printf("Added %s.\n", strings.ToUpper(s))
		} else {
			fmt.Printf("%s is already tracked.\n", strings.ToUpper(s))
		}
	}
	return nil
}

func cmdRemove(symbols []string) error {
	if len(symbols) == 0 {
		return fmt.Errorf("remove requires at least one symbol")
	}
	for _, s := range symbols {
		removed, err := store.RemoveTracker(s)
		if err != nil {
			return err
		}
		if removed {
			fmt.Printf("Removed %s.\n", strings.ToUpper(s))
		} else {
			fmt.Printf("%s was not in the tracker list.\n", strings.ToUpper(s))
		}
	}
	return nil
}

func cmdPosition(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: position <set|list|remove> ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		m, err := positions.Load()
		if err != nil {
			return err
		}
		if len(m) == 0 {
			fmt.Println("No positions recorded. Add one with: stock-tracker position set SLV 970 59.55")
			return nil
		}
		syms := make([]string, 0, len(m))
		for s := range m {
			syms = append(syms, s)
		}
		sort.Strings(syms)
		fmt.Println("Positions:")
		for _, s := range syms {
			p := m[s]
			fmt.Printf("  - %s: %g @ $%.2f  (cost $%.2f)\n", s, p.Qty, p.Avg, p.Qty*p.Avg)
		}
		return nil

	case "set":
		if len(rest) != 3 {
			return fmt.Errorf("usage: position set SYM QTY AVG  (e.g. position set SLV 970 59.55)")
		}
		sym := strings.ToUpper(rest[0])
		qty, err := strconv.ParseFloat(rest[1], 64)
		if err != nil || qty <= 0 {
			return fmt.Errorf("invalid QTY: %s", rest[1])
		}
		avg, err := strconv.ParseFloat(rest[2], 64)
		if err != nil || avg <= 0 {
			return fmt.Errorf("invalid AVG: %s", rest[2])
		}
		if err := positions.Set(sym, qty, avg); err != nil {
			return err
		}
		added, err := store.AddTracker(sym) // a position implies we want to monitor it
		if err != nil {
			return err
		}
		fmt.Printf("Set %s: %g @ $%.2f (cost $%.2f).\n", sym, qty, avg, qty*avg)
		if added {
			fmt.Printf("Also added %s to the tracker list.\n", sym)
		}
		return nil

	case "remove":
		if len(rest) != 1 {
			return fmt.Errorf("usage: position remove SYM")
		}
		sym := strings.ToUpper(rest[0])
		removed, err := positions.Remove(sym)
		if err != nil {
			return err
		}
		if removed {
			fmt.Printf("Removed position %s. (still tracked; use 'remove %s' to untrack)\n", sym, sym)
		} else {
			fmt.Printf("%s had no recorded position.\n", sym)
		}
		return nil

	default:
		return fmt.Errorf("unknown position subcommand: %s (use set|list|remove)", sub)
	}
}

func cmdPrice(symbols []string) error {
	if len(symbols) == 0 {
		return fmt.Errorf("price requires at least one symbol")
	}
	for _, s := range symbols {
		q, err := stock.GetQuote(strings.ToUpper(s))
		if err != nil {
			fmt.Printf("%s: error: %v\n", strings.ToUpper(s), err)
			continue
		}
		fmt.Printf("%s: %.2f (prev close %.2f, %+.2f%%)\n",
			q.Symbol, q.CurrentPrice, q.PreviousClose, q.ChangePct()*100)
	}
	return nil
}

func cmdResearch(symbols []string) error {
	if len(symbols) == 0 {
		return fmt.Errorf("research requires at least one symbol")
	}
	if !ai.Enabled() {
		return fmt.Errorf("research requires OPENAI_API_KEY to be set")
	}
	for _, s := range symbols {
		sym := strings.ToUpper(s)
		q, err := stock.GetQuote(sym)
		if err != nil {
			fmt.Printf("%s: error: %v\n", sym, err)
			continue
		}
		msg, err := ai.Research(sym, q.CurrentPrice, q.PreviousClose)
		if err != nil {
			fmt.Printf("%s: error: %v\n", sym, err)
			continue
		}
		fmt.Println(msg)
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
