// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nodescaler

import (
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"
	core "k8s.io/client-go/testing"
)

// readyNodeCondition for when a node is ready, as most in this test suite will be
var readyNodeCondition = []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}

func TestListNodePods(t *testing.T) {
	t.Parallel()
	fixture := &v1.PodList{Items: []v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "default"}}}}

	s, err := NewServer("", "app=game-server", "0.5", 5, time.Second)
	assert.Nil(t, err)
	sc := fake.NewSimpleClientset(fixture)
	s.cs = sc

	pods, err := s.listNodePods(v1.Node{})
	assert.Nil(t, err)
	assert.Equal(t, fixture, pods)
}

func TestSumCPUResourceRequests(t *testing.T) {
	t.Parallel()
	fixture := &v1.PodList{Items: []v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("0.5")}}}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "default"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("0.3")}}}}}}}}

	expected := resource.MustParse("0.8")
	assert.Equal(t, expected.MilliValue(), sumCPUResourceRequests(fixture))
}

func TestNewNodeList(t *testing.T) {
	t.Parallel()
	nodes := &v1.NodeList{Items: []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "foo", Labels: map[string]string{"app": "game-server"}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("4.0")}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "bar", Labels: map[string]string{"app": "game-server"}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("4.0")}}}}}

	pl1 := &v1.PodList{Items: []v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("0.5")}}}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "default"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("0.3")}}}}}}}}

	pl2 := &v1.PodList{Items: []v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod3", Namespace: "default"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1.2")}}}}}}}}

	cs := &fake.Clientset{}
	s, err := NewServer("", "app=game-server", "0.5", 5, time.Second)
	assert.Nil(t, err)
	s.cs = cs

	cs.AddReactor("list", "nodes", func(a core.Action) (bool, runtime.Object, error) {
		log.Print("Asking for list of nodes...")
		return true, nodes, nil
	})

	cs.AddReactor("list", "pods", func(a core.Action) (bool, runtime.Object, error) {
		var obj *v1.PodList
		la := a.(core.ListAction)

		switch la.GetListRestrictions().Fields.String() {
		case "spec.nodeName=foo":
			obj = pl1
		case "spec.nodeName=bar":
			obj = pl2
		}
		return true, obj, nil
	})

	nodeList, err := s.newNodeList()
	assert.Nil(t, err)
	assert.Equal(t, nodes, nodeList.nodes)
	assert.Equal(t, pl1, nodeList.nodePods(nodes.Items[0]))
	assert.Equal(t, pl2, nodeList.nodePods(nodes.Items[1]))
}

func TestNodeReady(t *testing.T) {
	t.Parallel()
	n := v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "foo"},
		Status: v1.NodeStatus{}}

	assert.False(t, nodeReady(n))

	n = v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "foo"},
		Status: v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionFalse}}}}
	assert.False(t, nodeReady(n))

	n = v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "foo"},
		Status: v1.NodeStatus{Conditions: readyNodeCondition}}
	assert.True(t, nodeReady(n))
}

func TestAvailableNodes(t *testing.T) {
	t.Parallel()
	nodes := &v1.NodeList{Items: []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "foo", Labels: map[string]string{"app": "game-server"}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1.0")},
				Conditions: readyNodeCondition}},
		{ObjectMeta: metav1.ObjectMeta{Name: "bar", Labels: map[string]string{"app": "game-server"}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("2.0")},
				Conditions: readyNodeCondition}},
		{ObjectMeta: metav1.ObjectMeta{Name: "goat", Labels: map[string]string{"app": "game-server"}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("3.0")},
				Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionFalse}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "unscheduled", Labels: map[string]string{"app": "game-server"}},
			Spec: v1.NodeSpec{Unschedulable: true},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("2.0")},
				Conditions: readyNodeCondition}}}}

	nl := nodeList{nodes: nodes}
	expected := []v1.Node{nodes.Items[0], nodes.Items[1]}
	an := nl.availableNodes()

	assert.Equal(t, expected, an)
}

func TestCpuRequestsAvailable(t *testing.T) {
	t.Parallel()
	nodes := &v1.NodeList{Items: []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "foo", Labels: map[string]string{"app": "game-server"}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("2.0")},
				Conditions: readyNodeCondition}},
		{ObjectMeta: metav1.ObjectMeta{Name: "bar", Labels: map[string]string{"app": "game-server"}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("2.0")},
				Conditions: readyNodeCondition}}}}

	pl1 := &v1.PodList{Items: []v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("0.5")}}}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "default"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("0.3")}}}}}}}}

	pl2 := &v1.PodList{Items: []v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod3", Namespace: "default"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1.8")}}}}}}}}

	cs := &fake.Clientset{}
	s, err := NewServer("", "app=game-server", "0.5", 5, time.Second)
	assert.Nil(t, err)
	s.cs = cs

	cs.AddReactor("list", "nodes", func(a core.Action) (bool, runtime.Object, error) {
		log.Print("Asking for list of nodes...")
		return true, nodes, nil
	})

	nl, err := s.newNodeList()
	assert.Nil(t, err)
	count := s.cpuRequestsAvailable(nl)
	assert.Equal(t, int64(8), count)

	cs.AddReactor("list", "pods", func(a core.Action) (bool, runtime.Object, error) {
		var obj *v1.PodList
		la := a.(core.ListAction)

		switch la.GetListRestrictions().Fields.String() {
		case "spec.nodeName=foo":
			obj = pl1
		case "spec.nodeName=bar":
			obj = pl2
		}
		return true, obj, nil
	})

	nl, err = s.newNodeList()
	assert.Nil(t, err)

	count = s.cpuRequestsAvailable(nl)
	assert.Equal(t, int64(2), count)
}

func TestNewGameWatcher(t *testing.T) {
	t.Parallel()

	mw := watch.NewFake()
	gw := &gameWatcher{events: make(chan bool), watcher: mw}
	gw.start()
	go func() {
		defer gw.stop()
		mw.Action(watch.Added, nil)
		mw.Action(watch.Deleted, nil)
		mw.Action(watch.Error, nil)
		mw.Action(watch.Modified, nil)
		mw.Action(watch.Deleted, nil)
	}()

	i := 0
	for range gw.events {
		i++
	}
	assert.Equal(t, 3, i)
}

func TestCordon(t *testing.T) {
	t.Parallel()

	nodes := &v1.NodeList{Items: []v1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "foo",
		Labels:      map[string]string{"app": "game-server"},
		Annotations: map[string]string{}}, Spec: v1.NodeSpec{Unschedulable: false},
		Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("2.0")},
			Conditions: readyNodeCondition}},
	}}
	cs := &fake.Clientset{}
	s, err := NewServer("", "app=game-server", "0.5", 5, time.Second)
	assert.Nil(t, err)
	s.cs = cs
	cs.AddReactor("list", "nodes", func(a core.Action) (bool, runtime.Object, error) {
		return true, nodes, nil
	})
	cs.AddReactor("update", "nodes", func(a core.Action) (bool, runtime.Object, error) {
		ua := a.(core.UpdateAction)
		n := ua.GetObject().(*v1.Node)
		nodes.Items[0] = *n
		return true, n, nil
	})

	now := time.Now().UTC()
	node := nodes.Items[0]
	err = s.cordon(&node, true)
	assert.Nil(t, err)
	assert.True(t, node.Spec.Unschedulable)
	var ts time.Time
	err = ts.UnmarshalText([]byte(node.ObjectMeta.Annotations[timestampAnnotation]))
	assert.Nil(t, err)
	assert.True(t, ts.Equal(now) || ts.After(now), "Now: %v is not equal to or after %v", now, ts)

	nl, err := s.newNodeList()
	assert.Nil(t, err)
	assert.True(t, nl.nodes.Items[0].Spec.Unschedulable)
	assert.Equal(t, 0, len(nl.availableNodes()))

	err = s.cordon(&node, false)
	assert.Nil(t, err)
	assert.False(t, node.Spec.Unschedulable)
	err = ts.UnmarshalText([]byte(node.ObjectMeta.Annotations[timestampAnnotation]))
	assert.Nil(t, err)
	assert.True(t, ts.Equal(now) || ts.After(now), "Now: %v is not equal to or after %v", now, ts)

	nl, err = s.newNodeList()
	assert.Nil(t, err)
	assert.False(t, nl.nodes.Items[0].Spec.Unschedulable)
	assert.Equal(t, 1, len(nl.availableNodes()))
}
