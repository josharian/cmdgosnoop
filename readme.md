To use, grab the commits from https://github.com/josharian/go/tree/toolexec-help and re-build cmd/go.

Then:

```bash
$ go get -u github.com/josharian/cmdgosnoop
$ go build -a -toolexec cmdgosnoop -o /dev/null cmd/go
$ curl http://localhost:10808/chart
```

Hit /chart promptly!
And then wait at least 15s before running again, to let the data collection daemon die.

It'll spit out the number of concurrent commands running and what they were.
The idea is to spot inefficiencies in how cmd/go schedules work,
and also (possibly, later) to spot bottlenecks in user package structure.

Plan is to put in some html instead of terminal dump and make more user friendly.
