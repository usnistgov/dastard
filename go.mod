module github.com/usnistgov/dastard

go 1.17

require internal/mysql v0.0.0

replace internal/mysql => ./internal/mysql

require internal/ringbuffer v0.0.0

replace internal/ringbuffer => ./internal/ringbuffer

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/fabiokung/shm v0.0.0-20150728212823-2852b0d79bae
	github.com/pebbe/zmq4 v1.2.9
	github.com/spf13/viper v1.15.0
	github.com/stretchr/testify v1.8.1
	gonum.org/v1/gonum v0.12.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
)

require (
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-sql-driver/mysql v1.7.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/afero v1.9.4 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	golang.org/x/sys v0.5.0 // indirect
	golang.org/x/text v0.7.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
