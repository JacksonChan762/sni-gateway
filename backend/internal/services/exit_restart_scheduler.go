package services

import (
	"log"
	"strings"

	"github.com/robfig/cron/v3"
)

type ExitRestartScheduler struct {
	cron    *cron.Cron
	manager *ExitManager
	spec    string
}

func NewExitRestartScheduler(manager *ExitManager, spec string) *ExitRestartScheduler {
	return &ExitRestartScheduler{
		cron:    cron.New(),
		manager: manager,
		spec:    strings.TrimSpace(spec),
	}
}

func (s *ExitRestartScheduler) Start() {
	if s == nil || s.manager == nil {
		return
	}
	if s.spec == "" || strings.EqualFold(s.spec, "disabled") {
		log.Println("Scheduled enabled exit restart disabled")
		return
	}

	if _, err := s.cron.AddFunc(s.spec, func() {
		log.Println("Scheduled enabled exit restart started")
		if err := s.manager.RestartEnabledExits(); err != nil {
			log.Printf("Scheduled enabled exit restart failed: %v", err)
			return
		}
		log.Println("Scheduled enabled exit restart completed")
	}); err != nil {
		log.Printf("Failed to schedule enabled exit restart %q: %v", s.spec, err)
		return
	}

	s.cron.Start()
	log.Printf("Enabled exit restart scheduled with cron %q", s.spec)
}

func (s *ExitRestartScheduler) Stop() {
	if s == nil || s.cron == nil {
		return
	}
	s.cron.Stop()
}
