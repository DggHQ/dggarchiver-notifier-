package state

import (
	"encoding/json"
	"log/slog"
	"os"
	"reflect"

	config "github.com/DggHQ/dggarchiver-config/notifier"
	dggarchivermodel "github.com/DggHQ/dggarchiver-model"
	"github.com/nats-io/nats.go"
)

type State struct {
	SearchETag     string
	SentVODs       []string
	CurrentStreams struct {
		YouTube dggarchivermodel.VOD
		Rumble  dggarchivermodel.VOD
		Kick    dggarchivermodel.VOD
	} `json:"-"`
	kv nats.KeyValue
}

func New(cfg *config.Config) State {
	var err error

	var js nats.JetStreamContext
	js, _ = cfg.NATS.NatsConnection.JetStream()

	var kv nats.KeyValue
	kv, err = js.KeyValue("dggarchiver-notifier")
	if err != nil {
		kv, err = js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:      "dggarchiver-notifier",
			Description: "KV store for the dggarchiver-notifier service.",
		})
		if err != nil {
			slog.Error("unable to create kv store", slog.Any("err", err))
			os.Exit(1)
		}
	}

	return State{
		SearchETag: "",
		SentVODs:   make([]string, 0),
		kv:         kv,
	}
}

func (state *State) CheckPriority(platformName string, config *config.Config) bool {
	stateValue := reflect.ValueOf(state.CurrentStreams)
	platformsValue := reflect.ValueOf(config.Notifier.Platforms)
	priority := platformsValue.FieldByName(platformName).FieldByName("Priority").Int()
	if priority <= 1 {
		return true
	}
	platformsFields := reflect.VisibleFields(reflect.TypeOf(config.Notifier.Platforms))
	for _, field := range platformsFields {
		if field.Name != platformName {
			if platformsValue.FieldByName(field.Name).FieldByName("Priority").Int() < priority && stateValue.FieldByName(field.Name).FieldByName("ID").String() != "" {
				return false
			}
		}
	}
	return false
}

func (state *State) Dump() {
	b, err := json.Marshal(state)
	if err != nil {
		slog.Error("unable to marshal state", slog.Any("err", err))
		os.Exit(1)
	}
	_, err = state.kv.Put("state", b)
	if err != nil {
		slog.Error("unable to put state", slog.Any("err", err))
		os.Exit(1)
	}
}

func (state *State) Load() {
	e, err := state.kv.Get("state")
	if err != nil {
		slog.Warn("unable to load state", slog.Any("err", err))
		return
	}
	err = json.Unmarshal(e.Value(), state)
	if err != nil {
		slog.Error("unable to unmarshal state", slog.Any("err", err))
		os.Exit(1)
	}
}
