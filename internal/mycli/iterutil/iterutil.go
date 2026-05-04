// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package iterutil

import "iter"

// ZipShortestBy pairs values from two sequences and stops when either sequence ends.
//
// This keeps the shorter-input semantics of go-iterator-helper's hiter.Pairs.
// It intentionally differs from samber/lo/it.ZipBy2, which pads missing values
// with zero values when the sequences have different lengths.
func ZipShortestBy[A, B, R any](as iter.Seq[A], bs iter.Seq[B], f func(A, B) R) iter.Seq[R] {
	return func(yield func(R) bool) {
		nextA, stopA := iter.Pull(as)
		defer stopA()
		nextB, stopB := iter.Pull(bs)
		defer stopB()

		for {
			a, ok := nextA()
			if !ok {
				return
			}
			b, ok := nextB()
			if !ok {
				return
			}
			if !yield(f(a, b)) {
				return
			}
		}
	}
}
