# Attribution and third-party software

The project's application code is distributed under the repository's MIT license. Dependencies retain their own licenses.

- **SPINACH research** — Shicheng Liu, Sina J. Semnani, Harold Triedman, Jialiang Xu, Isaac Dan Zhao and Monica S. Lam. [Paper](https://arxiv.org/abs/2407.11417), [original code](https://github.com/stanford-oval/spinach). This project is an independent educational adaptation. Its fictional sample graphs were created for the lab; the original SPINACH dataset is not redistributed here.
- **goRDFlib v0.9.3** — parser, SPARQL evaluator, RDF model/parsers, native SQLite RDF store and native HTTP endpoint. BSD-3-Clause; [included license](licenses/goRDFlib-LICENSE.txt), [upstream](https://github.com/tggo/goRDFlib).
- **modernc.org/sqlite v1.60.1** — CGO-free SQLite port/driver. [Included license](licenses/modernc-sqlite-LICENSE.txt), [package](https://pkg.go.dev/modernc.org/sqlite). Its transitive packages and bundled components have their own notices.
- **W3C RDF tests** — fetched separately for validation, pinned in the validation report. Consult the [test repository](https://github.com/w3c/rdf-tests) for its license and test-data terms. The full suite is not bundled with this project.
- **Jev / TypeSafe API** — external authenticated navigation service. [System One API](https://docs.typesafe.ai/api/system-one). Model weights are not redistributed.
- **OpenAI API** — external authenticated action-generation and synthesis service. [Responses API](https://developers.openai.com/api/reference/resources/responses/methods/create). Model weights are not redistributed.

No measured superiority over SPINACH, Fuseki, Neo4j or another engine is claimed. Attribution to the original research and third-party evaluator does not imply endorsement.
