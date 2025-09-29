#!/usr/bin/env python3
import sys
import numpy as np

# usage: test.py filename.npy number_of_records


def main():
    fname = sys.argv[1]
    d = np.load(fname)
    print(d)
    want = int(sys.argv[2])
    assert len(d) == want, f"{len(d)=}, want {want}"
    sys.exit(0)


if __name__ == "__main__":
    main()
