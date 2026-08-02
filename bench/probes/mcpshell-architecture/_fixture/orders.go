package inventory

import "errors"

var ErrOutOfStock = errors.New("out of stock")

// Reserve takes stock for an order. It reads then writes without holding a
// lock across both, so two concurrent Reserves can both see the same count.
func Reserve(s *Store, sku string, n int) error {
	if s.Get(sku) < n {
		return ErrOutOfStock
	}
	s.Add(sku, -n)
	return nil
}

func Restock(s *Store, sku string, n int) { s.Add(sku, n) }
