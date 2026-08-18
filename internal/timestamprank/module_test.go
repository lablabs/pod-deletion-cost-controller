package timestamprank_test

import (
	"errors"
	"testing"

	"github.com/lablabs/pod-deletion-cost-controller/internal/module"
	"github.com/lablabs/pod-deletion-cost-controller/internal/timestamprank"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type recordingRegistrator struct {
	registered []module.Handler
	err        error
}

func (r *recordingRegistrator) AddModule(h module.Handler) error {
	if r.err != nil {
		return r.err
	}
	r.registered = append(r.registered, h)
	return nil
}

func TestRegister(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name      string
		algoTypes []string
		want      bool
	}{
		{name: "explicitly enabled", algoTypes: []string{"timestamp-rank"}, want: true},
		{name: "enabled alongside others", algoTypes: []string{"zone", "timestamp-rank"}, want: true},
		{name: "empty list registers everything", algoTypes: nil, want: true},
		{name: "not in list", algoTypes: []string{"zone"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recordingRegistrator{}
			require.NoError(t, timestamprank.Register(log, r, nil, tt.algoTypes))

			if !tt.want {
				assert.Empty(t, r.registered)
				return
			}
			require.Len(t, r.registered, 1)
			assert.Equal(t, []string{"timestamp-rank"}, r.registered[0].AcceptType())
		})
	}
}

func TestRegister_PropagatesRegistrationError(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	r := &recordingRegistrator{err: errors.New("already registered")}

	err := timestamprank.Register(log, r, nil, []string{"timestamp-rank"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timestamp-rank")
	assert.Contains(t, err.Error(), "already registered")
}
