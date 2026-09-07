package appregistry

import (
	"fmt"
	"sync"

	haikunator "github.com/atrox/haikunatorgo/v2"
)

type AppRegistry struct {
	haikunator *haikunator.Haikunator
	apps       map[string]*App
	mutex      *sync.RWMutex
}

var once sync.Once
var appRegistry *AppRegistry

func (r *AppRegistry) addApp(app *App) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, ok := r.apps[app.AppId]; ok {
		return fmt.Errorf("app with id %s already exists", app.AppId)
	}

	r.apps[app.AppId] = app
	return nil
}

func (r *AppRegistry) GetApp(appId string) (*App, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	app, ok := r.apps[appId]

	return app, ok
}

func (r *AppRegistry) DeleteApp(appId string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.apps, appId)
}

func (r *AppRegistry) NewAppId() string {
	return r.haikunator.Haikunate()
}

func (r *AppRegistry) NewApp(appCfg AppConfig) (*App, error) {
	return newApp(appCfg)
}

func NewAppRegistry() *AppRegistry {
	once.Do(func() {
		h := haikunator.New()
		h.TokenLength = 5
		h.TokenChars = "abcdefghijklmnopqrstuvwxyz0123456789"

		appRegistry = &AppRegistry{
			apps:       make(map[string]*App),
			mutex:      &sync.RWMutex{},
			haikunator: h,
		}

	})

	return appRegistry
}
