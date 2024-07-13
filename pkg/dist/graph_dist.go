package dist

import "github.com/Garetonchick/distbuild/pkg/build"

type GraphDistributer struct {
	graph   *build.Graph
	adjList [][]int
}

func buildAdjucencyList(graph *build.Graph) [][]int {
	id2idx := make(map[build.ID]int)
	for i, job := range graph.Jobs {
		id2idx[job.ID] = i
	}

	al := make([][]int, len(graph.Jobs))

	for i, job := range graph.Jobs {
		for _, dep := range job.Deps {
			al[i] = append(al[i], id2idx[dep])
		}
	}
	return al
}

func NewGraphDistributer(graph *build.Graph) *GraphDistributer {
	return &GraphDistributer{
		graph:   graph,
		adjList: buildAdjucencyList(graph),
	}
}

func (d *GraphDistributer) DistributeGraph(doJob func(job *build.Job) error) error {
	// TODO: write concurrent implementation
	visited := make([]bool, len(d.adjList))

	var walk func(v int) error
	walk = func(v int) error {
		visited[v] = true
		for _, u := range d.adjList[v] {
			if !visited[u] {
				err := walk(u)
				if err != nil {
					return err
				}
			}
		}
		return doJob(&d.graph.Jobs[v])
	}

	for v := range len(d.adjList) {
		if !visited[v] {
			err := walk(v)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
