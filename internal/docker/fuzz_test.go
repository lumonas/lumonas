package docker

import "testing"

func FuzzComposeStructure(f *testing.F) {
	f.Add("services:\n  media:\n    image: example/media:latest\n")
	f.Add("services:\n")
	f.Fuzz(func(t *testing.T, compose string) {
		_ = validateComposeStructure(compose)
	})
}

func FuzzBuildCompose(f *testing.F) {
	f.Add("media", "example/media:latest", "/media")
	f.Fuzz(func(t *testing.T, name, image, containerPath string) {
		_, _ = BuildCompose(CatalogApp{Image: image, Ports: []int{8080}}, name, nil, []StorageMapping{{ResourceID: "share", ContainerPath: containerPath}})
	})
}
