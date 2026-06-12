package ensweb

import (
	"net/http"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
)

// func (s *Server) SetIndex(dirPath string) {
// 	s.mux.Handle("/", s.IndexRoute(dirPath)).Methods("GET")
// }

func (s *Server) AddRoute(path string, method string, hf HandlerFunc) {
	s.mux.Handle(path, basicHandleFunc(s, hf)).Methods(method)
}

func (s *Server) AddPrefixRoute(prefix string, dirpath string, hf HandlerFunc) {
	s.prefixPath = prefix
	s.publicPath = dirpath
	s.mux.PathPrefix(prefix).Handler(basicHandleFunc(s, hf))
}

func (s *Server) EnableSWagger(url string) {
	// Use a relative spec URL so the Swagger UI fetches doc.json from the same
	// origin the page was loaded on (localhost, 127.0.0.1, a LAN IP, etc.).
	// An absolute URL would hardcode the bind host (e.g. 0.0.0.0) and trip the
	// browser's cross-origin check when the page is opened on a different host.
	s.mux.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("./doc.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("none"),
		httpSwagger.DomID("swagger-ui"))).Methods(http.MethodGet)
}

// func (s *Server) SetStatic(dir string) {
// 	s.publicPath = dir
// 	s.mux.PathPrefix("/").Handler(http.FileServer(http.Dir(dir)))
// }

func (s *Server) SetStatic(prefix string, dir string) {
	s.publicPath = dir
	s.prefixPath = prefix
	s.mux.PathPrefix("/").Handler(indexRoute(s, dir))
}

func (s *Server) GetRouteVar(req *Request, key string) string {
	vars := mux.Vars(req.r)
	return vars[key]
}
