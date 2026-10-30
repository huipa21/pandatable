package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

func actorOf(user auth.User) structure.Actor {
	return structure.Actor{ID: user.ID, Locale: user.Locale, Timezone: user.EffectiveTimezone}
}

func (api *authHTTP) mountStructure(router chi.Router) {
	router.Get("/nav", api.authenticated(api.nav))
	router.Post("/workspaces", api.authenticated(api.createWorkspace))
	router.Patch("/workspaces/{id}", api.authenticated(api.updateWorkspace))
	router.Delete("/workspaces/{id}", api.authenticated(api.deleteWorkspace))
	router.Post("/workspaces/{id}/books", api.authenticated(api.createBook))
	router.Patch("/books/{id}", api.authenticated(api.updateBook))
	router.Delete("/books/{id}", api.authenticated(api.deleteBook))
	router.Post("/books/{id}/tables", api.authenticated(api.createTable))
	router.Patch("/tables/{id}", api.authenticated(api.updateTable))
	router.Delete("/tables/{id}", api.authenticated(api.deleteTable))
	router.Get("/tables/{id}/schema", api.authenticated(api.tableSchema))
	router.Get("/tables/{id}/directory/users", api.authenticated(api.tableUsers))
	router.Post("/tables/{id}/directory/user-labels", api.authenticated(api.tableUserLabels))
	router.Post("/tables/{id}/fields", api.authenticated(api.createField))
	router.Post("/formula:check", api.authenticated(api.checkFormula))
	router.Post("/tables/{id}/relations", api.authenticated(api.createRelation))
	router.Patch("/fields/{id}", api.authenticated(api.updateField))
	router.Delete("/fields/{id}", api.authenticated(api.deleteField))
	router.Post("/views/{id}/query", api.authenticated(api.queryRows))
	router.Post("/views/{id}/groups", api.authenticated(api.queryGroups))
	router.Post("/tables/{id}/views", api.authenticated(api.createView))
	router.Post("/views/{id}/duplicate", api.authenticated(api.duplicateView))
	router.Patch("/views/{id}", api.authenticated(api.updateView))
	router.Delete("/views/{id}", api.authenticated(api.deleteView))
	router.Post("/views/{id}/move", api.authenticated(api.moveView))
	router.Get("/tables/{id}/rows/{rowId}", api.authenticated(api.getRow))
	router.Post("/tables/{id}/rows/{rowId}/links/{fieldId}", api.authenticated(api.changeLinks))
	router.Post("/tables/{id}/rows", api.authenticated(api.createRows))
	router.Patch("/tables/{id}/rows", api.authenticated(api.patchRows))
	router.Patch("/tables/{id}/rows/{rowId}", api.authenticated(api.patchRow))
	router.Delete("/tables/{id}/rows", api.authenticated(api.deleteRows))
}

func (api *authHTTP) checkFormula(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		TableID string `json:"tableId"`
		Expr    string `json:"expr"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	if input.TableID == "" {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Formula tableId is required"))
		return
	}
	result, err := api.structure.CheckFormula(r.Context(), actorOf(user), input.TableID, input.Expr)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	if result.Position > 0 {
		writeJSON(w, http.StatusOK, struct {
			Error struct {
				Position int    `json:"position"`
				Message  string `json:"message"`
			} `json:"error"`
		}{Error: struct {
			Position int    `json:"position"`
			Message  string `json:"message"`
		}{result.Position, result.Message}})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Type string `json:"type"`
	}{Type: string(result.Type)})
}

func (api *authHTTP) nav(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.RawQuery != "" {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Navigation does not accept query parameters"))
		return
	}
	result, err := api.structure.Nav(r.Context(), actorOf(user))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func optionalText(raw json.RawMessage) (structure.OptionalText, error) {
	if len(raw) == 0 {
		return structure.OptionalText{}, nil
	}
	value, err := patchString(raw, true)
	if err != nil {
		return structure.OptionalText{}, err
	}
	return structure.OptionalText{Set: true, Value: value}, nil
}

func (api *authHTTP) createWorkspace(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Name  string          `json:"name"`
		Icon  json.RawMessage `json:"icon"`
		Color json.RawMessage `json:"color"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	icon, err := optionalText(input.Icon)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	color, err := optionalText(input.Color)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	workspace, err := api.structure.CreateWorkspace(r.Context(), actorOf(user),
		structure.WorkspaceInput{Name: input.Name, Icon: icon.Value, Color: color.Value})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Workspace structure.Workspace `json:"workspace"`
		EventID   string              `json:"eventId"`
	}{workspace, rand.Text()})
}

func (api *authHTTP) updateWorkspace(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Name  *string         `json:"name"`
		Icon  json.RawMessage `json:"icon"`
		Color json.RawMessage `json:"color"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	icon, err := optionalText(input.Icon)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	color, err := optionalText(input.Color)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	workspace, err := api.structure.UpdateWorkspace(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		structure.WorkspacePatch{Name: input.Name, Icon: icon, Color: color})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Workspace structure.Workspace `json:"workspace"`
		EventID   string              `json:"eventId"`
	}{workspace, rand.Text()})
}

func (api *authHTTP) deleteWorkspace(w http.ResponseWriter, r *http.Request, user auth.User) {
	if err := api.structure.DeleteWorkspace(r.Context(), actorOf(user), chi.URLParam(r, "id")); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": rand.Text()})
}

type namedInput struct {
	Name string `json:"name"`
}

type movableInput struct {
	Name  *string `json:"name"`
	Index *int    `json:"index"`
}

func (api *authHTTP) createBook(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input namedInput
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	book, err := api.structure.CreateBook(r.Context(), actorOf(user), chi.URLParam(r, "id"), input.Name)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Book    structure.Book `json:"book"`
		EventID string         `json:"eventId"`
	}{book, rand.Text()})
}

func (api *authHTTP) updateBook(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input movableInput
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	book, err := api.structure.UpdateBook(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		structure.BookPatch{Name: input.Name, Index: input.Index})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Book    structure.Book `json:"book"`
		EventID string         `json:"eventId"`
	}{book, rand.Text()})
}

func (api *authHTTP) deleteBook(w http.ResponseWriter, r *http.Request, user auth.User) {
	if err := api.structure.DeleteBook(r.Context(), actorOf(user), chi.URLParam(r, "id")); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": rand.Text()})
}

func (api *authHTTP) createTable(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input namedInput
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	schema, err := api.structure.CreateTable(r.Context(), actorOf(user), chi.URLParam(r, "id"), input.Name)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		structure.TableSchema
		EventID string `json:"eventId"`
	}{schema, rand.Text()})
}

func (api *authHTTP) updateTable(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Name           *string `json:"name"`
		Index          *int    `json:"index"`
		DisplayFieldID *string `json:"displayFieldId"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	table, err := api.structure.UpdateTable(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		structure.TablePatch{Name: input.Name, Index: input.Index, DisplayFieldID: input.DisplayFieldID})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Table   structure.Table `json:"table"`
		EventID string          `json:"eventId"`
	}{table, rand.Text()})
}

func (api *authHTTP) deleteTable(w http.ResponseWriter, r *http.Request, user auth.User) {
	if err := api.structure.DeleteTable(r.Context(), actorOf(user), chi.URLParam(r, "id")); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": rand.Text()})
}

func (api *authHTTP) tableSchema(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.RawQuery != "" {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Table schema does not accept query parameters"))
		return
	}
	schema, err := api.structure.Schema(r.Context(), actorOf(user), chi.URLParam(r, "id"))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, schema)
}
