package main

import "errors"

var ErrConfigPathNotSet = errors.New("CONFIG_PATH not set and -config flag not provided")
