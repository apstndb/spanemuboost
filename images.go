package spanemuboost

// Runtime image references are maintained in runtime-images.json. Generation
// is deterministic and offline; registry discovery is a separate mise task.
//go:generate go run ./internal/cmd/genimages
