package management

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	kitlog "github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
	"github.com/micromdm/micromdm/workflow"
	"golang.org/x/net/context"
)

var errBadUUID = errors.New("request must have a valid uuid")

// ServiceHandler returns an HTTP Handler for the management service
func ServiceHandler(ctx context.Context, svc Service, logger kitlog.Logger) http.Handler {
	opts := []kithttp.ServerOption{
		kithttp.ServerErrorLogger(logger),
		kithttp.ServerErrorEncoder(encodeError),
	}

	fetchDEPHandler := kithttp.NewServer(
		ctx,
		makeFetchDevicesEndpoint(svc),
		decodeFetchDEPDevicesRequest,
		encodeResponse,
		opts...,
	)

	addProfileHandler := kithttp.NewServer(
		ctx,
		makeAddProfileEndpoint(svc),
		decodeAddProfileRequest,
		encodeResponse,
		opts...,
	)
	listProfilesHandler := kithttp.NewServer(
		ctx,
		makeListProfilesEndpoint(svc),
		decodeListProfilesRequest,
		encodeResponse,
		opts...,
	)
	showProfileHandler := kithttp.NewServer(
		ctx,
		makeShowProfileEndpoint(svc),
		decodeShowProfileRequest,
		encodeResponse,
		opts...,
	)
	deleteProfileHandler := kithttp.NewServer(
		ctx,
		makeDeleteProfileEndpoint(svc),
		decodeDeleteProfileRequest,
		encodeResponse,
		opts...,
	)

	r := mux.NewRouter()

	r.Handle("/management/v1/devices/fetch", fetchDEPHandler).Methods("POST")
	r.Handle("/management/v1/profiles", addProfileHandler).Methods("POST")
	r.Handle("/management/v1/profiles", listProfilesHandler).Methods("GET")
	r.Handle("/management/v1/profiles/{uuid}", showProfileHandler).Methods("GET")
	r.Handle("/management/v1/profiles/{uuid}", deleteProfileHandler).Methods("DELETE")

	return r
}

func decodeFetchDEPDevicesRequest(_ context.Context, r *http.Request) (interface{}, error) {
	return fetchDEPDevicesRequest{}, nil
}

func decodeAddProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var request addProfileRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err == io.EOF {
		return nil, errEmptyRequest
	}
	if request.PayloadIdentifier == "" {
		return nil, errEmptyRequest
	}
	return request, err
}

func decodeListProfilesRequest(_ context.Context, r *http.Request) (interface{}, error) {
	return listProfilesRequest{}, nil
}

func decodeShowProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	vars := mux.Vars(r)
	uuid, ok := vars["uuid"]
	if !ok {
		return nil, errBadRouting
	}
	// simple validation
	if len(uuid) != 36 {
		return nil, errBadUUID
	}
	return showProfileRequest{UUID: uuid}, nil
}

func decodeDeleteProfileRequest(_ context.Context, r *http.Request) (interface{}, error) {
	vars := mux.Vars(r)
	uuid, ok := vars["uuid"]
	if !ok {
		return nil, errBadRouting
	}
	// simple validation
	if len(uuid) != 36 {
		return nil, errBadUUID
	}
	return deleteProfileRequest{UUID: uuid}, nil
}

func encodeResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// for success responses
	if e, ok := response.(statuser); ok {
		w.WriteHeader(e.status())
		if e.status() == http.StatusNoContent {
			return nil
		}
	}

	// check if this is a collection
	if e, ok := response.(listEncoder); ok {
		return e.encodeList(w)

	}
	return json.NewEncoder(w).Encode(response)
}

type errorer interface {
	error() error
}

type statuser interface {
	status() int
}

type listEncoder interface {
	encodeList(w http.ResponseWriter) error
}

// encode errors from business-logic
func encodeError(_ context.Context, err error, w http.ResponseWriter) {
	// unwrap if the error is wrapped by kit http in it's own error type
	if httperr, ok := err.(kithttp.Error); ok {
		err = httperr.Err
	}
	switch err {
	case ErrNotFound:
		w.WriteHeader(http.StatusNotFound)
	case errEmptyRequest, errBadUUID:
		w.WriteHeader(http.StatusBadRequest)
	case workflow.ErrExists:
		w.WriteHeader(http.StatusConflict)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": err.Error(),
	})
}
