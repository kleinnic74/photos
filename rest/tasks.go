package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"

	"bitbucket.org/kleinnic74/photos/rest/cursor"
	"bitbucket.org/kleinnic74/photos/tasks"
	"github.com/gorilla/mux"
)

type TaskHandler struct {
	tasks    *tasks.TaskRepository
	executor tasks.TaskExecutor
}

func NewTaskHandler(repo *tasks.TaskRepository, executor tasks.TaskExecutor) *TaskHandler {
	return &TaskHandler{tasks: repo, executor: executor}
}

func (h *TaskHandler) InitRoutes(r *mux.Router) {
	r.HandleFunc("/taskdefinitions", h.getTaskDefinitions).Methods("GET").Name("/taskdefinitions")
	r.HandleFunc("/tasks", h.postTask).Methods("POST").Name("/tasks")
	r.HandleFunc("/tasks", h.listTasks).Methods("GET").Name("/tasks")
}

func (h *TaskHandler) getTaskDefinitions(w http.ResponseWriter, r *http.Request) {
	defined := h.tasks.DefinedTasks()
	Respond(r).WithJSON(w, http.StatusOK, &simplePayload{Data: defined})
}

type task struct {
	Type       string          `json:"type"`
	Parameters json.RawMessage `json:"parameters"`
}

func (h *TaskHandler) postTask(w http.ResponseWriter, r *http.Request) {
	responder := Respond(r)
	task, err := parseTask(h.tasks, r.Body)
	if err != nil {
		responder.WithError(w, http.StatusBadRequest, err)
		return
	}
	execution, err := h.executor.Submit(r.Context(), task)
	if err != nil {
		responder.WithError(w, http.StatusServiceUnavailable, err)
	}
	responder.WithJSON(w, http.StatusAccepted, execution)
}

func (h *TaskHandler) listTasks(w http.ResponseWriter, r *http.Request) {
	t := h.executor.ListTasks(r.Context())
	sort.Sort(tasks.ExecutionsBySubmission(t))
	Respond(r).WithJSON(w, http.StatusOK, cursor.Unpaged(t))
}

func parseTask(repo *tasks.TaskRepository, in io.Reader) (t tasks.Task, err error) {
	var tmp task
	err = json.NewDecoder(in).Decode(&tmp)
	if err != nil {
		return
	}
	t, err = repo.CreateTask(tmp.Type)
	if err != nil {
		return
	}
	json.Unmarshal(tmp.Parameters, t)
	return
}
