package main

import (
	"encoding/json"
	"fmt"
	"go_shed/src/dag"
	"go_shed/src/db"
	"go_shed/src/models"
	_ "go_shed/src/user"
	"net/http"
	"os"
	"sync"

	"github.com/rs/zerolog/log"
)

type SharedState struct {
	sync.Mutex
	ActiveEndpoints  int
	ShouldBeShutdown bool
}

func main() {
	cfg := ParseConfig()
	cfg.setupZerolog()
	var ss SharedState
	dbClient, err := db.NewClient("/Users/ds/GoProjects/go_sched/test.db")
	if err != nil {
		log.Panic().Err(err).Msg("Cannot connect to the database")
	}
	start(dbClient)

	// Endpoints
	http.HandleFunc("/dag/list", ss.ListDagsHandler)
	http.HandleFunc("/task/next", ss.NextTaskHandler)
	http.HandleFunc("/shutdown", ss.ShutdownHandler)

	log.Info().Msgf("Start Scheduler v%s on :%d...", cfg.AppVersion, cfg.Port)
	lasErr := http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), nil)
	if lasErr != nil {
		log.Panic().Err(lasErr).Msg("Cannot start the server")
	}
}

func (ss *SharedState) ListDagsHandler(w http.ResponseWriter, r *http.Request) {
	ss.StartEndpoint()
	defer ss.CheckIfCanSafelyExit()
	defer ss.FinishEndpoint()
	dags := dag.List()
	fmt.Fprintf(w, "%s", string(dags[0].Id))
}

func (ss *SharedState) NextTaskHandler(w http.ResponseWriter, r *http.Request) {
	ss.StartEndpoint()
	defer ss.CheckIfCanSafelyExit()
	defer ss.FinishEndpoint()
	tte := models.TaskToExec{DagId: "hello_dag", TaskId: "say_hello"}
	tteJson, err := json.Marshal(tte)
	if err != nil {
		fmt.Fprint(w, "Error - could not json.Marshal TaskToExec")
		return
	}
	fmt.Fprint(w, string(tteJson))
}

func (ss *SharedState) ShutdownHandler(w http.ResponseWriter, r *http.Request) {
	ss.Lock()
	ss.ShouldBeShutdown = true
	log.Warn().Msg("Shutdown signal registered")
	ss.Unlock()
}

func (ss *SharedState) StartEndpoint() {
	ss.Lock()
	ss.ActiveEndpoints += 1
	ss.Unlock()
}

func (ss *SharedState) FinishEndpoint() {
	ss.Lock()
	ss.ActiveEndpoints -= 1
	ss.Unlock()
}

func (ss *SharedState) CheckIfCanSafelyExit() {
	ss.Lock()
	if ss.ShouldBeShutdown && ss.ActiveEndpoints == 0 {
		os.Exit(0)
	}
	ss.Unlock()
}

// JUST FOR INIT TEST
type testTask struct {
	TaskId string
}

func (tt testTask) Id() string { return tt.TaskId }
func (tt testTask) Execute()   { fmt.Println(tt.TaskId) }
