package app

import "testing"

func TestEnvEnsureTags(t *testing.T) {
	st := EnvState{Environments: []Environment{
		{ID: "1", Name: "HCCM - Test"},
		{ID: "2", Name: "工作流-Local"},
		{ID: "3", Name: "Prod"},
	}}
	got := st.EnsureTags()
	if got.Environments[0].Tag != "HCCM" || got.Environments[1].Tag != "工作流" || got.Environments[2].Tag != "" {
		t.Fatalf("tags = %#v", got.Environments)
	}
}
