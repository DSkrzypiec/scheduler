package dag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dskrzypiec/scheduler/timeutils"
)

const LOG_PREFIX = "dag"

var ErrTaskNotFoundInDag = errors.New("task was not found in the DAG")

// TODO: docs
type Dag struct {
	Id       Id
	Schedule *Schedule
	Attr     Attr
	Root     *Node
}

type Attr struct {
	// If set to true schedule dag run would be catch up since the last run or
	// Start.
	CatchUp bool     `json:"catchUp"`
	Tags    []string `json:"tags"`
}

func New(id Id) *Dag {
	return &Dag{
		Id: id,
	}
}

func (d *Dag) AddRoot(node *Node) *Dag {
	d.Root = node
	return d
}

func (d *Dag) AddSchedule(sched Schedule) *Dag {
	d.Schedule = &sched
	return d
}

func (d *Dag) AddAttributes(attr Attr) *Dag {
	d.Attr = attr
	return d
}

func (d *Dag) Done() Dag {
	return *d
}

// Graph is a valid DAG when the following conditions are met:
//   - Is acyclic (does not have cycles)
//   - Task identifiers are unique within the graph
//   - Graph is no deeper then MAX_RECURSION
func (d *Dag) IsValid() bool {
	return d.Root.isAcyclic() && d.Root.taskIdsUnique() &&
		d.Root.depth() <= MAX_RECURSION
}

// GetTask return task by its identifier. In case when there is no Task within
// the DAG of given taskId, then non-nil error will be returned
// (ErrTaskNotFoundInDag).
func (d *Dag) GetTask(taskId string) (Task, error) {
	nodesInfo := d.Root.Flatten()
	for _, ni := range nodesInfo {
		if ni.Node.Task.Id() == taskId {
			return ni.Node.Task, nil
		}
	}
	return nil, ErrTaskNotFoundInDag
}

// Flatten DAG into list of Tasks in BFS order.
func (d *Dag) Flatten() []Task {
	if d.Root == nil {
		return []Task{}
	}
	nodesInfo := d.Root.Flatten()
	tasks := make([]Task, len(nodesInfo))
	for idx, ni := range nodesInfo {
		tasks[idx] = ni.Node.Task
	}
	return tasks
}

// FlattenNodes flatten DAG into list of Nodes with enriched information in BFS
// order.
func (d *Dag) FlattenNodes() []NodeInfo {
	if d.Root == nil {
		return []NodeInfo{}
	}
	return d.Root.Flatten()
}

// TaskParents returns mapping of DAG task IDs onto its parents task IDs.
func (d *Dag) TaskParents() map[string][]string {
	if d.Root == nil {
		return map[string][]string{}
	}
	_, parentsNodeMap := d.Root.flattenBFS() // This does not include the root
	taskParents := make(map[string][]string, len(parentsNodeMap))
	taskParents[d.Root.Task.Id()] = []string{}

	for node, nodeParents := range parentsNodeMap {
		if node == nil {
			// This should not happen
			continue
		}
		parentTaskIds := make([]string, 0, len(nodeParents))
		for _, parent := range nodeParents {
			if parent != nil {
				parentTaskIds = append(parentTaskIds, parent.Task.Id())
			}
		}
		taskParents[node.Task.Id()] = parentTaskIds
	}
	return taskParents
}

// HashAttr calculates SHA256 hash based on DAG attribues, start time and
// schedule.
func (d *Dag) HashDagMeta() string {
	attrJson, jErr := json.Marshal(d.Attr)
	if jErr != nil {
		slog.Error("Cannot serialize DAG attributes", "attr", d.Attr, "err",
			jErr)
		return "CANNOT SERIALIZE DAG ATTRIBUTES"
	}
	sched := ""
	startTsStr := ""
	if d.Schedule != nil {
		sched = (*d.Schedule).String()
		startTsStr = timeutils.ToString((*d.Schedule).StartTime())
	}

	hasher := sha256.New()
	hasher.Write(attrJson)
	hasher.Write([]byte(sched))
	hasher.Write([]byte(startTsStr))
	return hex.EncodeToString(hasher.Sum(nil))
}

// HashTasks calculates SHA256 hash based on concatanated body sources of
// Execute methods for all tasks.
func (d *Dag) HashTasks() string {
	if d.Root == nil {
		hasher := sha256.New()
		hasher.Write([]byte("NO TASKS"))
		return hex.EncodeToString(hasher.Sum(nil))
	}
	return d.Root.Hash()
}

func (d *Dag) String() string {
	return fmt.Sprintf("Dag: %s (%s)\nTasks:\n%s", d.Id,
		(*d.Schedule).String(), d.Root.String(0))
}
