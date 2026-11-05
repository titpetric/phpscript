package stdlib

// Owned maps each name to the first area that defines it, one table per kind,
// because a function, a class and a constant may share a name.
type Owned struct {
	Funcs, Classes, Constants map[string]string
}

// Owners maps each name to the first area that defines it.
func Owners(infos []AreaInfo) Owned {
	o := Owned{Funcs: map[string]string{}, Classes: map[string]string{}, Constants: map[string]string{}}
	for _, info := range infos {
		for _, p := range []struct {
			into  map[string]string
			names []string
		}{{o.Funcs, info.Funcs}, {o.Classes, info.Classes}, {o.Constants, info.Constants}} {
			for _, name := range p.names {
				if _, ok := p.into[name]; !ok {
					p.into[name] = info.Name
				}
			}
		}
	}
	return o
}
