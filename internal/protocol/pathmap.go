package protocol

import "strings"

// PathMapping maps a host path to a container path for transparent translation
// when Locus runs in a container with different mount points.
type PathMapping struct {
	Host      string
	Container string
}

// PathMapper translates between host and container paths.
type PathMapper struct {
	mappings []PathMapping
}

// NewPathMapper creates a PathMapper from a spec string.
// Format: "host1:container1,host2:container2" (comma-separated host:container pairs).
func NewPathMapper(spec string) *PathMapper {
	pm := &PathMapper{}
	if spec == "" {
		return pm
	}
	for _, pair := range strings.Split(spec, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			pm.mappings = append(pm.mappings, PathMapping{
				Host:      strings.TrimSpace(parts[0]),
				Container: strings.TrimSpace(parts[1]),
			})
		}
	}
	return pm
}

// ToContainer converts a host path to the container path.
func (pm *PathMapper) ToContainer(hostPath string) string {
	for _, m := range pm.mappings {
		if strings.HasPrefix(hostPath, m.Host) {
			return m.Container + hostPath[len(m.Host):]
		}
	}
	return hostPath
}

// ToHost converts a container path to the host path.
func (pm *PathMapper) ToHost(containerPath string) string {
	for _, m := range pm.mappings {
		if strings.HasPrefix(containerPath, m.Container) {
			return m.Host + containerPath[len(m.Container):]
		}
	}
	return containerPath
}
