package sim

import (
	"fmt"
	"math/big"
	"strings"
)

type StoreError struct{ msg string }

func (e *StoreError) Error() string { return e.msg }

type BoundsError struct{ msg string }

func (e *BoundsError) Error() string { return e.msg }

type undoEntry func()

// store is the in-memory state with an undo log per step (spec §4).
type store struct {
	g       *Graph
	undo    []undoEntry
	inStep  bool
	counter int
	before  map[string]*big.Rat
	changes int
}

func newStore(g *Graph) *store { return &store{g: g} }

func (s *store) conservedTotals() map[string]*big.Rat {
	out := map[string]*big.Rat{}
	for _, a := range s.g.accounts {
		if !s.g.vtype(a.ValueType).Conserved {
			continue
		}
		if out[a.ValueType] == nil {
			out[a.ValueType] = new(big.Rat)
		}
		out[a.ValueType].Add(out[a.ValueType], a.Value)
	}
	return out
}

func (s *store) begin() error {
	if s.inStep {
		return &StoreError{"nested step"}
	}
	s.inStep, s.undo, s.changes = true, nil, 0
	s.before = s.conservedTotals()
	return nil
}

// totalsDiff lists the conserved types whose total changed, in name order; empty when none
// did. A type with no account totals 0, so opening its first account at 0 changes nothing.
func totalsDiff(before, after map[string]*big.Rat) string {
	keys := map[string]bool{}
	for _, m := range []map[string]*big.Rat{before, after} {
		for k := range m {
			keys[k] = true
		}
	}
	total := func(m map[string]*big.Rat, k string) *big.Rat {
		if v := m[k]; v != nil {
			return v
		}
		return ratZero
	}
	var diff strings.Builder
	for _, k := range sortedKeys(keys) {
		if d := new(big.Rat).Sub(total(after, k), total(before, k)); d.Sign() != 0 {
			fmt.Fprintf(&diff, " %s: %s", k, numString(d))
		}
	}
	return diff.String()
}

func (s *store) commit() error {
	if diff := totalsDiff(s.before, s.conservedTotals()); diff != "" {
		return &BoundsError{"conserved totals changed within step:" + diff}
	}
	s.inStep, s.undo = false, nil
	return nil
}

func (s *store) rollback() {
	for i := len(s.undo) - 1; i >= 0; i-- {
		s.undo[i]()
	}
	s.undo, s.inStep = nil, false
}

func (s *store) needStep() error {
	if !s.inStep {
		return &StoreError{"change outside a step"}
	}
	return nil
}

func (s *store) actor(id string) (*Actor, error) {
	a := s.g.actorIdx[id]
	if a == nil {
		return nil, &StoreError{"unknown actor " + id}
	}
	return a, nil
}

func (s *store) account(actor, name string) *Account { return s.g.accIdx[accKey{actor, name}] }

func (s *store) setField(id, key string, value any) error {
	if err := s.needStep(); err != nil {
		return err
	}
	a, err := s.actor(id)
	if err != nil {
		return err
	}
	old, existed := a.Data.Get(key)
	s.undo = append(s.undo, func() {
		if existed {
			a.Data.Set(key, old)
		} else {
			a.Data.Delete(key)
		}
	})
	a.Data.Set(key, value)
	s.changes++
	return nil
}

func (s *store) createActor(typ, title string, data *OMap, createdBy, logical string) (string, error) {
	if err := s.needStep(); err != nil {
		return "", err
	}
	s.counter++
	id := fmt.Sprintf("sim-%d", s.counter)
	for s.g.actorIdx[id] != nil {
		s.counter++
		id = fmt.Sprintf("sim-%d", s.counter)
	}
	d := deepCopy(data).(*OMap)
	if !d.Has("_logical_id") {
		d.Set("_logical_id", logical)
	}
	s.g.addActor(&Actor{ID: id, Type: typ, Title: title, Data: d, CreatedBy: createdBy})
	s.undo = append(s.undo, func() { s.g.removeActor(id) })
	s.changes++
	return id, nil
}

func (s *store) createLink(src, dst, edgeType string) (string, error) {
	if err := s.needStep(); err != nil {
		return "", err
	}
	if _, err := s.actor(src); err != nil {
		return "", err
	}
	if _, err := s.actor(dst); err != nil {
		return "", err
	}
	id := fmt.Sprintf("simlink-%d", len(s.g.links)+1)
	for s.g.linkIdx[id] != nil {
		id += "x"
	}
	s.g.addLink(&Link{ID: id, Source: src, Target: dst, EdgeType: edgeType})
	s.undo = append(s.undo, func() { s.g.removeLink(id) })
	s.changes++
	return id, nil
}

func (s *store) ensureAccount(actor, name, valueType string) (*Account, error) {
	if a := s.account(actor, name); a != nil {
		return a, nil
	}
	if err := s.needStep(); err != nil {
		return nil, err
	}
	if _, err := s.actor(actor); err != nil {
		return nil, err
	}
	a := &Account{ActorID: actor, Name: name, ValueType: valueType, Value: new(big.Rat)}
	s.g.addAccount(a)
	k := accKey{actor, name}
	s.undo = append(s.undo, func() { s.g.removeAccount(k) })
	s.changes++
	return a, nil
}

func (s *store) setValue(a *Account, v *big.Rat) error {
	vt := s.g.vtype(a.ValueType)
	q, err := vt.quantize(v)
	if err != nil {
		return err
	}
	if err := vt.checkBounds(q, a.ActorID+"."+a.Name); err != nil {
		return &BoundsError{err.Error()}
	}
	old := a.Value
	s.undo = append(s.undo, func() { a.Value = old })
	a.Value = q
	return nil
}

func (s *store) transfer(src, dst accKey, amount *big.Rat) error {
	if err := s.needStep(); err != nil {
		return err
	}
	a := s.account(src.actor, src.name)
	if a == nil {
		return &StoreError{fmt.Sprintf("no account %s.%s", src.actor, src.name)}
	}
	b := s.account(dst.actor, dst.name)
	if b == nil {
		var err error
		if b, err = s.ensureAccount(dst.actor, dst.name, a.ValueType); err != nil {
			return err
		}
	}
	if a.ValueType != b.ValueType {
		return &StoreError{fmt.Sprintf("transfer between different value types %s -> %s", a.ValueType, b.ValueType)}
	}
	q, err := s.g.vtype(a.ValueType).quantize(amount)
	if err != nil {
		return err
	}
	if q.Sign() < 0 {
		return &StoreError{"transfer amount must be >= 0"}
	}
	if err := s.setValue(a, new(big.Rat).Sub(a.Value, q)); err != nil {
		return err
	}
	if err := s.setValue(b, new(big.Rat).Add(b.Value, q)); err != nil {
		return err
	}
	s.changes++
	return nil
}

func (s *store) add(target accKey, amount *big.Rat, valueType string) error {
	if err := s.needStep(); err != nil {
		return err
	}
	a, err := s.accountOrNew(target, valueType)
	if err != nil {
		return err
	}
	if s.g.vtype(a.ValueType).Conserved {
		return &StoreError{fmt.Sprintf("%s is conserved: use transfer, not add (%s.%s)", a.ValueType, target.actor, target.name)}
	}
	s.changes++
	return s.setValue(a, new(big.Rat).Add(a.Value, amount))
}

func (s *store) setAccount(target accKey, value *big.Rat, valueType string) error {
	if err := s.needStep(); err != nil {
		return err
	}
	a, err := s.accountOrNew(target, valueType)
	if err != nil {
		return err
	}
	if s.g.vtype(a.ValueType).Conserved {
		return &StoreError{fmt.Sprintf("%s is conserved: use transfer (%s.%s)", a.ValueType, target.actor, target.name)}
	}
	s.changes++
	return s.setValue(a, value)
}

func (s *store) accountOrNew(k accKey, valueType string) (*Account, error) {
	if a := s.account(k.actor, k.name); a != nil {
		return a, nil
	}
	if valueType == "" {
		valueType = "default"
	}
	return s.ensureAccount(k.actor, k.name, valueType)
}
