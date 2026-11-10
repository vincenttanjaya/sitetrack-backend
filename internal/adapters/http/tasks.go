package httpapi

import (
	"net/http"

	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (r *Router) RegisterTaskRoutes(service *application.TaskService) {
	const tasks = "/v1/projects/{projectID}/tasks"
	const task = tasks + "/{taskID}"
	const dependencies = task + "/dependencies"
	const dependency = dependencies + "/{prerequisiteID}"
	r.authenticated("GET "+tasks, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		q, err := parseTaskList(req)
		if err != nil {
			return err
		}
		result, err := service.List(req.Context(), actor, req.PathValue("projectID"), q)
		return writeTaskResult(w, http.StatusOK, result, err)
	})
	r.authenticated("POST "+tasks, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		var input domain.TaskInput
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		result, err := service.Create(req.Context(), actor, req.PathValue("projectID"), input)
		return writeTaskResult(w, http.StatusCreated, result, err)
	})
	r.authenticated("GET "+task, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		result, err := service.Detail(req.Context(), actor, req.PathValue("projectID"), req.PathValue("taskID"))
		return writeTaskResult(w, http.StatusOK, result, err)
	})
	r.authenticated("PUT "+task, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		var input domain.TaskInput
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		result, err := service.Update(req.Context(), actor, req.PathValue("projectID"), req.PathValue("taskID"), input)
		return writeTaskResult(w, http.StatusOK, result, err)
	})
	r.authenticated("DELETE "+task, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		if err := service.Delete(req.Context(), actor, req.PathValue("projectID"), req.PathValue("taskID")); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	r.authenticated("POST "+task+"/actions/{action}", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		result, err := service.Act(req.Context(), actor, req.PathValue("projectID"), req.PathValue("taskID"), domain.TaskAction(req.PathValue("action")))
		return writeTaskResult(w, http.StatusOK, result, err)
	})
	r.authenticated("POST "+dependencies, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		var input domain.DependencyInput
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		result, err := service.AddDependency(req.Context(), actor, req.PathValue("projectID"), req.PathValue("taskID"), input.PrerequisiteID)
		return writeTaskResult(w, http.StatusCreated, result, err)
	})
	r.authenticated("GET "+dependency, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		result, err := service.Dependency(req.Context(), actor, req.PathValue("projectID"), req.PathValue("taskID"), req.PathValue("prerequisiteID"))
		return writeTaskResult(w, http.StatusOK, result, err)
	})
	r.authenticated("PUT "+dependency, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		var input domain.DependencyInput
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		result, err := service.ReplaceDependency(req.Context(), actor, req.PathValue("projectID"), req.PathValue("taskID"), req.PathValue("prerequisiteID"), input.PrerequisiteID)
		return writeTaskResult(w, http.StatusOK, result, err)
	})
	r.authenticated("DELETE "+dependency, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		if err := service.DeleteDependency(req.Context(), actor, req.PathValue("projectID"), req.PathValue("taskID"), req.PathValue("prerequisiteID")); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func parseTaskList(req *http.Request) (domain.TaskListQuery, error) {
	var query domain.TaskListQuery
	values, err := parseQuery(req, "q", "status", "late", "blocked", "from", "to", "sort", "order", "limit", "cursor")
	if err != nil {
		return query, err
	}
	query.Page, err = parsePage(values)
	if err != nil {
		return query, err
	}
	query.Late, err = parseBool(values, "late")
	if err != nil {
		return query, err
	}
	query.Blocked, err = parseBool(values, "blocked")
	if err != nil {
		return query, err
	}
	for _, name := range []string{"status", "from", "to", "sort", "order"} {
		if values.Has(name) && values.Get(name) == "" {
			return query, domain.Invalid(name + " must not be empty")
		}
	}
	query.Search = values.Get("q")
	query.Status = domain.TaskStatus(values.Get("status"))
	query.From = values.Get("from")
	query.To = values.Get("to")
	query.Sort = values.Get("sort")
	query.Order = values.Get("order")
	return query, nil
}

func writeTaskResult(w http.ResponseWriter, status int, result any, err error) error {
	if err != nil {
		return err
	}
	return writeJSON(w, status, result)
}
