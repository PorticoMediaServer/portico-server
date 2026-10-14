package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const scratchDirectory = "/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/scale-check-2"
const scratchLimit int64 = 8_000_000_000

// Leave headroom for one bounded writer batch and SQLite housekeeping.
const scratchStop int64 = 7_000_000_000

func scratchUsage() (int64, error) {
	out, err := exec.Command("du", "-s", "-B1", scratchDirectory).Output()
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 1 {
		return 0, fmt.Errorf("empty scratch size")
	}
	return strconv.ParseInt(fields[0], 10, 64)
}

var lastScratchCheck int64

func checkScratchBudget() error {
	bytes, err := scratchUsage()
	if err != nil {
		return err
	}
	if bytes >= scratchStop {
		return fmt.Errorf("scratch headroom exhausted: %d bytes (8 GB hard limit)", bytes)
	}
	lastScratchCheck = bytes
	return nil
}
