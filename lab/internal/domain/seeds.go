package domain

import "fmt"

func Seeds() []Graph {
	makeGraph := func(id, title, desc string, ns [][3]string, es [][3]string, questions []Example) Graph {
		g := Graph{ID: id, Title: title, Description: desc, Nodes: []Node{}, Edges: []Edge{}, Subgraphs: []Subgraph{}, Examples: questions}
		for i, n := range ns {
			g.Nodes = append(g.Nodes, Node{ID: Namespace + n[0], Label: n[1], Kind: "iri", Group: n[2], X: 110 + float64(i%4)*220, Y: 100 + float64(i/4)*150})
		}
		for i, e := range es {
			g.Edges = append(g.Edges, Edge{ID: fmt.Sprintf("e%d", i+1), Source: Namespace + e[0], Predicate: Namespace + e[1], Label: e[1], Target: Namespace + e[2]})
		}
		return g
	}
	science := makeGraph("aurora", "Laboratorio Aurora", "Un pequeño universo de ciencia: personas, disciplinas y ciudades. Datos ficticios para experimentar sin miedo.", [][3]string{
		{"elena", "Elena Torres", "Personas"}, {"sofia", "Sofía Vega", "Personas"}, {"lina", "Lina Costa", "Personas"}, {"mateo", "Mateo Ríos", "Personas"},
		{"fisica", "Física", "Disciplinas"}, {"biologia", "Biología", "Disciplinas"}, {"astronomia", "Astronomía", "Disciplinas"},
		{"aurora", "Instituto Aurora", "Instituciones"}, {"horizonte", "Centro Horizonte", "Instituciones"}, {"orbita", "Observatorio Órbita", "Instituciones"},
		{"paris", "París", "Ciudades"}, {"buenosaires", "Buenos Aires", "Ciudades"}, {"roma", "Roma", "Ciudades"},
	}, [][3]string{
		{"elena", "investiga", "fisica"}, {"elena", "trabajaEn", "aurora"}, {"sofia", "investiga", "fisica"}, {"sofia", "trabajaEn", "horizonte"},
		{"lina", "investiga", "biologia"}, {"lina", "trabajaEn", "aurora"}, {"mateo", "investiga", "astronomia"}, {"mateo", "trabajaEn", "orbita"},
		{"aurora", "ubicadoEn", "paris"}, {"horizonte", "ubicadoEn", "buenosaires"}, {"orbita", "ubicadoEn", "roma"},
		{"elena", "colaboraCon", "sofia"}, {"elena", "colaboraCon", "lina"}, {"sofia", "colaboraCon", "mateo"},
	}, []Example{
		{"¿Quién investiga física y trabaja en París?", "SELECT DISTINCT ?persona ?centro WHERE {\n  ?persona ex:investiga ex:fisica .\n  ?persona ex:trabajaEn ?centro .\n  ?centro ex:ubicadoEn ex:paris .\n}"},
		{"¿En qué ciudad trabaja cada persona?", "SELECT ?persona ?ciudad WHERE {\n  ?persona ex:trabajaEn ?centro .\n  ?centro ex:ubicadoEn ?ciudad .\n}"},
		{"¿Con quién colabora Elena?", "SELECT ?persona WHERE {\n  ex:elena ex:colaboraCon ?persona .\n}"},
	})
	science.Subgraphs = []Subgraph{
		{"ciencia", "Investigación", "Personas, disciplinas científicas e instituciones", ids("elena", "sofia", "lina", "mateo", "fisica", "biologia", "astronomia", "aurora", "horizonte", "orbita")},
		{"geografia", "Geografía", "Instituciones y las ciudades donde están ubicadas", ids("aurora", "horizonte", "orbita", "paris", "buenosaires", "roma")},
		{"colaboracion", "Colaboración", "Red de colaboración entre las personas", ids("elena", "sofia", "lina", "mateo")},
	}
	books := makeGraph("biblioteca", "Biblioteca de mundos", "Autores, libros y géneros de una biblioteca imaginaria. Seguí las relaciones para descubrir nuevas lecturas.", [][3]string{
		{"ines", "Inés Sol", "Autores"}, {"leo", "Leo Mar", "Autores"}, {"nora", "Nora Luz", "Autores"}, {"jardin", "El jardín azul", "Libros"}, {"mareas", "Las mareas", "Libros"}, {"estacion", "Última estación", "Libros"}, {"bosque", "El bosque de cristal", "Libros"}, {"fantasia", "Fantasía", "Géneros"}, {"cienciaficcion", "Ciencia ficción", "Géneros"}, {"aventura", "Aventura", "Géneros"},
	}, [][3]string{
		{"ines", "escribio", "jardin"}, {"ines", "escribio", "bosque"}, {"leo", "escribio", "mareas"}, {"nora", "escribio", "estacion"}, {"jardin", "genero", "fantasia"}, {"bosque", "genero", "fantasia"}, {"mareas", "genero", "aventura"}, {"estacion", "genero", "cienciaficcion"}, {"ines", "recomienda", "estacion"}, {"leo", "recomienda", "jardin"},
	}, []Example{{"¿Quién escribió libros de fantasía?", "SELECT DISTINCT ?autor ?libro WHERE {\n  ?autor ex:escribio ?libro .\n  ?libro ex:genero ex:fantasia .\n}"}, {"¿Qué libro recomienda Leo?", "SELECT ?libro WHERE { ex:leo ex:recomienda ?libro . }"}})
	books.Subgraphs = []Subgraph{{"autoria", "Autoría", "Autores y los libros que escribieron", ids("ines", "leo", "nora", "jardin", "mareas", "estacion", "bosque")}, {"generos", "Géneros", "Libros y géneros literarios", ids("jardin", "mareas", "estacion", "bosque", "fantasia", "cienciaficcion", "aventura")}}
	space := makeGraph("cosmos", "Misión Cosmos", "Un mapa de misiones y destinos inventados. Practicá consultas con varios saltos en un escenario espacial.", [][3]string{
		{"alba", "Misión Alba", "Misiones"}, {"nova", "Misión Nova", "Misiones"}, {"iris", "Misión Iris", "Misiones"}, {"marte", "Marte", "Destinos"}, {"luna", "Luna", "Destinos"}, {"europa", "Europa", "Destinos"}, {"rover", "Rover", "Instrumentos"}, {"camara", "Cámara", "Instrumentos"}, {"sensor", "Sensor de hielo", "Instrumentos"}, {"agencia", "Agencia Aurora", "Agencias"}, {"consorcio", "Consorcio Sur", "Agencias"},
	}, [][3]string{
		{"alba", "destino", "marte"}, {"nova", "destino", "luna"}, {"iris", "destino", "europa"}, {"alba", "lleva", "rover"}, {"alba", "lleva", "camara"}, {"nova", "lleva", "camara"}, {"iris", "lleva", "sensor"}, {"agencia", "opera", "alba"}, {"agencia", "opera", "iris"}, {"consorcio", "opera", "nova"},
	}, []Example{{"¿Qué agencia opera misiones que llevan una cámara?", "SELECT ?agencia ?mision WHERE {\n  ?agencia ex:opera ?mision .\n  ?mision ex:lleva ex:camara .\n}"}, {"¿A dónde viaja cada misión?", "SELECT ?mision ?destino WHERE { ?mision ex:destino ?destino . }"}})
	space.Subgraphs = []Subgraph{{"rutas", "Rutas", "Misiones y sus destinos", ids("alba", "nova", "iris", "marte", "luna", "europa")}, {"equipamiento", "Equipamiento", "Misiones e instrumentos científicos", ids("alba", "nova", "iris", "rover", "camara", "sensor")}, {"operadores", "Operadores", "Agencias que operan las misiones", ids("alba", "nova", "iris", "agencia", "consorcio")}}
	return []Graph{science, books, space}
}
func ids(local ...string) []string {
	out := []string{}
	for _, id := range local {
		out = append(out, Namespace+id)
	}
	return out
}
