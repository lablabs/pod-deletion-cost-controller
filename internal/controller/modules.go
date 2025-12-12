package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/lablabs/pod-deletion-cost-controller/internal/module"
	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

func NewModuleManager() *Manager {
	m := Manager{
		modules: make(map[string]module.Handler),
	}
	return &m
}

type Manager struct {
	modules map[string]module.Handler
}

func (m *Manager) AddModule(module module.Handler) error {
	for _, t := range module.AcceptType() {
		if _, exists := m.modules[t]; exists {
			return fmt.Errorf("module [%s] is already registered", t)
		}
		m.modules[t] = module
	}
	return nil
}

func (m *Manager) Handle(log logr.Logger, pod *v1.Pod, dep *v2.Deployment) error {
	algType := GetType(dep)
	if !IsEnabled(dep) {
		return nil
	}
	h, exist := m.modules[algType]
	if !exist {
		log.V(3).WithValues("deployment", dep.Name, TypeAnnotation, algType).Info("handler not found")
		return nil
	}
	return h.Handle(context.Background(), log, pod, dep)
}
