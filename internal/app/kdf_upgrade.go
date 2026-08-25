package app

import (
	"context"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type passwordHashUpgradeJob struct {
	server   *Server
	userID   string
	oldHash  string
	password []byte
	key      string
}

var passwordHashUpgrades = struct {
	once    sync.Once
	mu      sync.Mutex
	pending map[string]struct{}
	queue   chan passwordHashUpgradeJob
}{pending: map[string]struct{}{}, queue: make(chan passwordHashUpgradeJob, 16)}

func schedulePasswordHashUpgrade(server *Server, userID, oldHash, password string) {
	if server == nil || userID == "" || oldHash == "" {
		return
	}
	key := userID + "\x00" + oldHash
	passwordHashUpgrades.mu.Lock()
	if _, exists := passwordHashUpgrades.pending[key]; exists {
		passwordHashUpgrades.mu.Unlock()
		return
	}
	passwordHashUpgrades.pending[key] = struct{}{}
	passwordHashUpgrades.mu.Unlock()

	job := passwordHashUpgradeJob{server: server, userID: userID, oldHash: oldHash, password: append([]byte(nil), password...), key: key}
	select {
	case passwordHashUpgrades.queue <- job:
		passwordHashUpgrades.once.Do(func() { go runPasswordHashUpgrades() })
	default:
		passwordHashUpgrades.mu.Lock()
		delete(passwordHashUpgrades.pending, key)
		passwordHashUpgrades.mu.Unlock()
		clear(job.password)
	}
}

func runPasswordHashUpgrades() {
	for job := range passwordHashUpgrades.queue {
		func() {
			defer func() {
				clear(job.password)
				passwordHashUpgrades.mu.Lock()
				delete(passwordHashUpgrades.pending, job.key)
				passwordHashUpgrades.mu.Unlock()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			replacement, err := runKDF(ctx, kdfPasswordUpgradeHash, kdfLaneMutation, func() ([]byte, error) {
				return bcrypt.GenerateFromPassword(job.password, currentPasswordBcryptCost)
			})
			if err != nil {
				return
			}
			_, _ = job.server.execUserWrite(ctx, `
				UPDATE users SET password_hash = ?, updated_at = ?
				WHERE id = ? AND password_hash = ?`, string(replacement), time.Now().UTC().Format(time.RFC3339Nano), job.userID, job.oldHash)
		}()
	}
}
