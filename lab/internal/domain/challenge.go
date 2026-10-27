package domain

import (
	"fmt"
	"math"
	"math/rand"
)

// ChallengeGraph is a deterministic fictional fixture, not benchmark evidence.
func ChallengeGraph() Graph {
	g := Graph{ID: "research-network", Title: "Red de investigación", Description: "Un universo ficticio más amplio para comparar exploración con y sin Jev. Las preguntas combinan países, proyectos, financiación y publicaciones; ninguna medición aislada demuestra una mejora.", Nodes: []Node{}, Edges: []Edge{}}
	groups := []string{"Personas", "Instituciones", "Geografía", "Proyectos", "Publicaciones", "Financiación"}
	members := make(map[string][]string)
	node := func(id, label, group string) {
		index := 0
		for i, name := range groups {
			if name == group {
				index = i
			}
		}
		angle := float64(index) * math.Pi / 3
		offset := float64(len(members[group])) * .72
		g.Nodes = append(g.Nodes, Node{ID: Namespace + id, Label: label, Kind: "iri", Group: group, X: 550 + 330*math.Cos(angle) + 85*math.Cos(offset), Y: 350 + 225*math.Sin(angle) + 80*math.Sin(offset)})
		members[group] = append(members[group], Namespace+id)
	}
	edge := func(source, predicate, target string) {
		g.Edges = append(g.Edges, Edge{ID: fmt.Sprintf("challenge-%d", len(g.Edges)+1), Source: Namespace + source, Predicate: Namespace + predicate, Label: predicate, Target: Namespace + target})
	}
	person := func(i int) string { return fmt.Sprintf("r%02d", i+1) }
	institution := func(i int) string { return fmt.Sprintf("lab%d", i) }
	project := func(i int) string { return fmt.Sprintf("project%02d", i+1) }
	paper := func(i int) string { return fmt.Sprintf("paper%02d", i+1) }
	names := []string{"Ava Mora", "Bruno Paz", "Celia Sol", "Diego Mar", "Eva Lago", "Félix Roca", "Gala Luz", "Hugo Valle", "Iris Alba", "Julián Río", "Kiara Sur", "Leo Prado", "Mara Vega", "Nico Costa", "Olivia Monte", "Pablo Cielo", "Quimey Vidal", "Renata Luna", "Simón Nube", "Tania Vidal", "Uma Campo", "Vera Roble", "Waldo Cruz", "Zoe Flores"}
	for i, name := range names {
		node(person(i), name, "Personas")
	}
	for i, name := range []string{"Aurora Institute", "Meridian Center", "Orion Observatory", "Southern Institute", "Andes Lab", "Delta Center", "Horizon Institute", "Pacific Lab"} {
		node(institution(i), name, "Instituciones")
	}
	for i, name := range []string{"Paris", "Buenos Aires", "Rome", "Santiago", "Lyon", "Córdoba"} {
		node(fmt.Sprintf("city%d", i), name, "Geografía")
	}
	for i, name := range []string{"France", "Argentina", "Italy", "Chile"} {
		node(fmt.Sprintf("country%d", i), name, "Geografía")
	}
	for i, name := range []string{"Climate", "Robotics", "Astronomy", "Renewable energy"} {
		node(fmt.Sprintf("topic%d", i), name, "Proyectos")
	}
	for i := range 12 {
		node(project(i), fmt.Sprintf("Project %02d", i+1), "Proyectos")
	}
	for i, name := range []string{"Aurora Foundation", "Meridian Council", "South Trust"} {
		node(fmt.Sprintf("funder%d", i), name, "Financiación")
	}
	node("published", "Published", "Publicaciones")
	node("retracted", "Retracted", "Publicaciones")
	for i := range 18 {
		node(paper(i), fmt.Sprintf("Paper %02d", i+1), "Publicaciones")
		id := fmt.Sprintf("citations%d", i)
		g.Nodes = append(g.Nodes, Node{ID: id, Label: fmt.Sprint((i%6 + 1) * 10), Kind: "literal", Datatype: "http://www.w3.org/2001/XMLSchema#integer", Group: "Publicaciones", X: 100 + float64(i%6)*120, Y: 650 + float64(i/6)*75})
		members["Publicaciones"] = append(members["Publicaciones"], id)
		g.Edges = append(g.Edges, Edge{ID: fmt.Sprintf("challenge-%d", len(g.Edges)+1), Source: Namespace + paper(i), Predicate: Namespace + "citations", Label: "citations", Target: id})
		edge(paper(i), "aboutProject", project(i%12))
		status := "published"
		if i == 5 || i == 11 {
			status = "retracted"
		}
		edge(paper(i), "status", status)
	}
	for i := range 24 {
		edge(person(i), "worksAt", institution(i%8))
		edge(person(i), "specializesIn", fmt.Sprintf("topic%d", i%4))
		edge(person(i), "participatesIn", project(i%12))
		edge(person(i), "participatesIn", project((i+4)%12))
		edge(person(i), "collaboratesWith", person((i+1)%24))
		edge(person(i), "authored", paper(i%18))
		if i < 12 {
			edge(person(i), "leads", project(i))
		}
		if i%3 == 0 {
			edge(person((i+1)%24), "authored", paper(i%18))
			edge(person((i+1)%24), "participatesIn", project(i%12))
		}
	}
	for i := range 12 {
		edge(project(i), "topic", fmt.Sprintf("topic%d", i%4))
		edge(project(i), "fundedBy", fmt.Sprintf("funder%d", i%3))
		edge(project(i), "partner", institution(i%8))
		edge(project(i), "partner", institution((i+3)%8))
	}
	for i := range 8 {
		edge(institution(i), "locatedIn", fmt.Sprintf("city%d", i%6))
		if i < 7 {
			edge(institution(i), "partnersWith", institution(i+1))
		}
	}
	for i, country := range []int{0, 1, 2, 3, 0, 1} {
		edge(fmt.Sprintf("city%d", i), "country", fmt.Sprintf("country%d", country))
	}
	// Interleave regions reproducibly instead of placing useful edges at a chosen offset.
	random := rand.New(rand.NewSource(42))
	random.Shuffle(len(g.Edges), func(i, j int) { g.Edges[i], g.Edges[j] = g.Edges[j], g.Edges[i] })
	sub := func(id, name, description string, selectedGroups ...string) {
		selected := []string{}
		for _, group := range selectedGroups {
			selected = append(selected, members[group]...)
		}
		g.Subgraphs = append(g.Subgraphs, Subgraph{ID: id, Name: name, Description: description, Nodes: selected})
	}
	sub("people", "Investigadores", "Researcher affiliations, specialties and direct collaboration; worksAt, specializesIn, collaboratesWith.", "Personas", "Instituciones")
	sub("geography", "Geografía", "Institution locations and countries; locatedIn, country.", "Instituciones", "Geografía")
	sub("projects", "Proyectos", "Project leadership, participation and scientific topics; leads, participatesIn, topic.", "Personas", "Proyectos")
	sub("funding", "Financiación", "Project funders and partner institutions; fundedBy, partner.", "Proyectos", "Financiación", "Instituciones")
	sub("publications", "Publicaciones", "Authorship, publication status, citations and project evidence; authored, status, citations, aboutProject.", "Personas", "Publicaciones", "Proyectos")
	sub("partnerships", "Alianzas", "Directed institution partnerships; partnersWith, with institution locations.", "Instituciones", "Geografía")
	g.Examples = []Example{
		{"¿Qué proyectos de clima financiados por Aurora Foundation tienen investigadores en Francia, y quiénes son?", `SELECT DISTINCT ?project ?researcher WHERE {
  ?project ex:topic ex:topic0 ; ex:fundedBy ex:funder0 .
  ?researcher ex:participatesIn ?project ; ex:worksAt ?lab .
  ?lab ex:locatedIn/ex:country ex:country0 .
} ORDER BY ?researcher`},
		{"¿Qué proyectos reúnen investigadores de al menos dos países y cuántos países tiene cada uno?", `SELECT ?project (COUNT(DISTINCT ?country) AS ?countries) WHERE {
  ?researcher ex:participatesIn ?project ; ex:worksAt ?lab .
  ?lab ex:locatedIn/ex:country ?country .
} GROUP BY ?project HAVING (COUNT(DISTINCT ?country) >= 2) ORDER BY ?project`},
		{"¿Qué investigadores de Argentina lideran proyectos de energía renovable y no tienen publicaciones retractadas?", `SELECT DISTINCT ?researcher ?project WHERE {
  ?researcher ex:worksAt ?lab ; ex:leads ?project .
  ?lab ex:locatedIn/ex:country ex:country1 .
  ?project ex:topic ex:topic3 .
  FILTER NOT EXISTS { ?researcher ex:authored ?paper . ?paper ex:status ex:retracted . }
} ORDER BY ?researcher`},
		{"¿Qué instituciones llegan a Pacific Lab mediante una o más alianzas y emplean a un líder de proyectos de clima?", `SELECT DISTINCT ?lab WHERE {
  ?lab ex:partnersWith+ ex:lab7 .
  ?researcher ex:worksAt ?lab ; ex:leads ?project .
  ?project ex:topic ex:topic0 .
} ORDER BY ?lab`},
		{"¿Qué proyecto suma más citas de publicaciones vigentes con autores en Francia, sin contar dos veces un artículo?", `SELECT ?project (SUM(?citations) AS ?totalCitations) WHERE {
  { SELECT DISTINCT ?paper ?project ?citations WHERE {
      ?paper ex:aboutProject ?project ; ex:status ex:published ; ex:citations ?citations .
      ?researcher ex:authored ?paper ; ex:worksAt ?lab .
      ?lab ex:locatedIn/ex:country ex:country0 .
  } }
} GROUP BY ?project ORDER BY DESC(?totalCitations) ?project LIMIT 1`},
		{"¿Quién colabora con alguien de otro país, comparte un proyecto con esa persona y también firma una publicación vigente en común?", `SELECT DISTINCT ?researcher ?colleague WHERE {
  ?researcher ex:collaboratesWith ?colleague ; ex:worksAt ?lab ; ex:participatesIn ?project ; ex:authored ?paper .
  ?colleague ex:worksAt ?otherLab ; ex:participatesIn ?project ; ex:authored ?paper .
  ?lab ex:locatedIn/ex:country ?country . ?otherLab ex:locatedIn/ex:country ?otherCountry .
  ?paper ex:status ex:published . FILTER (?country != ?otherCountry)
} ORDER BY ?researcher`},
	}
	return g
}
