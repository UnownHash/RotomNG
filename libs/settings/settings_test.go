package settings

import "testing"

type testSettings struct {
	Value int
}

func (testSettings) Validate() error {
	return nil
}

type testConfig struct {
	*Container[testSettings]
}

func TestContainer(t *testing.T) {
	container, err := NewContainer(testSettings{Value: 42})
	if err != nil {
		t.Fatal(err)
	}

	cfg := testConfig{Container: container}
	cfgCopy := cfg

	getSettings := cfgCopy.GetSettings

	if v := cfg.GetSettings().Value; v != 42 {
		t.Errorf("got %d, expected 42", v)
	}
	if v := cfgCopy.GetSettings().Value; v != 42 {
		t.Errorf("got %d, expected 42", v)
	}

	cfg.PutSettings(testSettings{Value: 4242})

	if v := cfg.GetSettings().Value; v != 4242 {
		t.Errorf("got %d, expected 4242", v)
	}
	if v := cfgCopy.GetSettings().Value; v != 4242 {
		t.Errorf("got %d, expected 4242", v)
	}
	if v := getSettings().Value; v != 4242 {
		t.Errorf("got %d, expected 4242", v)
	}
}
