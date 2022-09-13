#!/usr/bin/python3
import argparse
import sys
import time


def main(argv):
    parser = argparse.ArgumentParser(description="Create logs for integration testing")
    # Add more options if you like
    parser.add_argument("-i", "--iterations", dest="iterations", type=str, default="1000",
                        help="Number of iterations the code will run")
    parser.add_argument("-p", "--prefix", dest="prefix", type=str, default="pre",
                        help="Prefix of all messages")
    parser.add_argument("-t", "--time", dest="time", type=str, default="0",
                        help="Sleeping time (sec) between prints")
    args = parser.parse_args(argv)
    for i in range(int(args.iterations)):
        print("{0}_{1}".format(args.prefix, i))
        time.sleep(int(args.time))


if __name__ == "__main__":
    main(sys.argv[1:])
