
# A Guide to Writing `slog` Handlers

This document is maintained by Jonathan Amsterdam `jba@google.com`.


# Contents

%toc


# Introduction

The standard library’s `log/slog` package has a two-part design.
A "frontend," implemented by the `Logger` type,
gathers structured log information like a message, level, and attributes,
and passes them to a "backend," an implementation of the `Handler` interface.
The package comes with two built-in handlers that should usually be adequate.
But you may need to write your own handler, and that is not always straightforward.
This guide is here to help.


# Loggers and their handlers

Writing a handler requires an understanding of how the `Logger` and `Handler`
types work together.

Each logger contains a handler. Certain `Logger` methods do some preliminary work,
such as gathering key-value pairs into `Attr`s, and then call one or more
`Handler` methods. These `Logger` methods are `With`, `WithGroup`,
and the output methods like `Info`, `Error` and so on.

An output method fulfills the main role of a logger: producing log output.
Here is a call to an output method:

    logger.Info("hello", "key", value)

There are two general output methods, `Log`, and `LogAttrs`. For convenience,
there is an output method for each of four common levels (`Debug`, `Info`,
`Warn` and `Error`), and corresponding methods that take a context (`DebugContext`,
`InfoContext`, `WarnContext` and `ErrorContext`).

Each `Logger` output method first calls its handler's `Enabled` method. If that call
returns true, the method constructs a `Record` from its arguments and calls
the handler's `Handle` method.

As a convenience and an optimization, attributes can be added to
`Logger` by calling the `With` method:

    logger = logger.With("k", v)

This call creates a new `Logger` value with the argument attributes; the
original remains unchanged.
All subsequent output from `logger` will include those attributes.
A logger's `With` method calls its handler's `WithAttrs` method.

The `WithGroup` method is used to avoid avoid key collisions in large programs
by establishing separate namespaces.
This call creates a new `Logger` value with a group named "g":

    logger = logger.WithGroup("g")

All subsequent keys for `logger` will be qualified by the group name "g".
Exactly what "qualified" means depends on how the logger's handler formats the
output.
The built-in `TextHandler` treats the group as a prefix to the key, separated by
a dot: `g.k` for a key `k`, for example.
The built-in `JSONHandler` uses the group as a key for a nested JSON object:

    {"g": {"k": v}}

A logger's `WithGroup` method calls its handler's `WithGroup` method.


# Implementing `Handler` methods

We can now talk about the four `Handler` methods in detail.
Along the way, we will write a handler that formats logs using a format
reminiscent of YAML. It will display this log output call:

    logger.Info("hello", "key", 23)

something like this:

    time: 2023-05-15T16:29:00
    level: INFO
    message: "hello"
    key: 23
    ---

Although this particular output is valid YAML,
our implementation doesn't consider the subtleties of YAML syntax,
so it will sometimes produce invalid YAML.
For example, it doesn't quote keys that have colons in them.
We'll call it `IndentHandler` to forestall disappointment.

We begin with the `IndentHandler` type
and the `New` function that constructs it from an `io.Writer` and options:

%include indenthandler1/indent_handler.go types -

We'll support only one option, the ability to set a minimum level in order to
suppress detailed log output.
Handlers should always declare this option to be a `slog.Leveler`.
The `slog.Leveler` interface is implemented by both `Level` and `LevelVar`.
A `Level` value is easy for the user to provide,
but changing the level of multiple handlers requires tracking them all.
If the user instead passes a `LevelVar`, then a single change to that `LevelVar`
will change the behavior of all handlers that contain it.
Changes to `LevelVar`s are goroutine-safe.

You might also consider adding a `ReplaceAttr` option to your handler,
like the [one for the built-in
handlers](https://pkg.go.dev/log/slog#HandlerOptions.ReplaceAttr).
Although `ReplaceAttr` will complicate your implementation, it will also
make your handler more generally useful.

The mutex will be used to ensure that writes to the `io.Writer` happen atomically.
Unusually, `IndentHandler` holds a pointer to a `sync.Mutex` rather than holding a
`sync.Mutex` directly.
There is a good reason for that, which we'll explain [later](#getting-the-mutex-right).

Our handler will need additional state to track calls to `WithGroup` and `WithAttrs`.
We will describe that state when we get to those methods.

## The `Enabled` method

The `Enabled` method is an optimization that can avoid unnecessary work.
A `Logger` output method will call `Enabled` before it processes any of its arguments,
to see if it should proceed.

The signature is

    Enabled(context.Context, Level) bool

The context is available to allow decisions based on contextual information.
For example, a custom HTTP request header could specify a minimum level,
which the server adds to the context used for processing that request.
A handler's `Enabled` method could report whether the argument level
is greater than or equal to the context value, allowing the verbosity
of the work done by each request to be controlled independently.

Our `IndentHandler` doesn't use the context. It just compares the argument level
with its configured minimum level:

%include indenthandler1/indent_handler.go enabled -

## The `Handle` method

The `Handle` method is passed a `Record` containing all the information to be
logged for a single call to a `Logger` output method.
The `Handle` method should deal with it in some way.
One way is to output the `Record` in some format, as `TextHandler` and `JSONHandler` do.
But other options are to modify the `Record` and pass it on to another handler,
enqueue the `Record` for later processing, or ignore it.

The signature of `Handle` is

    Handle(context.Context, Record) error

The context is provided to support applications that provide logging information
along the call chain. In a break with usual Go practice, the `Handle` method
should not treat a canceled context as a signal to stop work.

If `Handle` processes its `Record`, it should follow the rules in the
[documentation](https://pkg.go.dev/log/slog#Handler.Handle).
For example, a zero `Time` field should be ignored, as should zero `Attr`s.

A `Handle` method that is going to produce output should carry out the following steps:

1. Allocate a buffer, typically a `[]byte`, to hold the output.
It's best to construct the output in memory first,
then write it with a single call to `io.Writer.Write`,
to minimize interleaving with other goroutines using the same writer.

2. Format the special fields: time, level, message, and source location (PC).
As a general rule, these fields should appear first and are not nested in
groups established by `WithGroup`.

3. Format the result of `WithGroup` and `WithAttrs` calls.

4. Format the attributes in the `Record`.

5. Output the buffer.

That is how `IndentHandler.Handle` is structured:

%include indenthandler1/indent_handler.go handle -

The first line allocates a `[]byte` that should be large enough for most log
output.
Allocating a buffer with some initial, fairly large capacity is a simple but
significant optimization: it avoids the repeated copying and allocation that
happen when the initial slice is empty or small.
We'll return to this line in the section on [speed](#speed)
and show how we can do even better.

The next part of our `Handle` method formats the special attributes,
observing the rules to ignore a zero time and a zero PC.

Next, the method processes the result of `WithAttrs` and `WithGroup` calls.
We'll skip that for now.

Then it's time to process the attributes in the argument record.
We use the `Record.Attrs` method to iterate over the attributes
in the order the user passed them to the `Logger` output method.
Handlers are free to reorder or de-duplicate the attributes,
but ours does not.

Lastly, after adding the line "---" to the output to separate log records,
our handler makes a single call to `h.out.Write` with the buffer we've accumulated.
We hold the lock for this write to make it atomic with respect to other
goroutines that may be calling `Handle` at the same time.

At the heart of the handler is the `appendAttr` method, responsible for
formatting a single attribute:

%include indenthandler1/indent_handler.go appendAttr -

It begins by resolving the attribute, to run the `LogValuer.LogValue` method of
the value if it has one. All handlers should resolve every attribute they
process.

Next, it follows the handler rule that says that empty attributes should be
ignored.

Then it switches on the attribute kind to determine what format to use. For most
kinds (the default case of the switch), it relies on `slog.Value`'s `String` method to
produce something reasonable. It handles strings and times specially:
strings by quoting them, and times by formatting them in a standard way.

When `appendAttr` sees a `Group`, it calls itself recursively on the group's
attributes, after applying two more handler rules.
First, a group with no attributes is ignored&mdash;not even its key is displayed.
Second, a group with an empty key is inlined: the group boundary isn't marked in
any way. In our case, that means the group's attributes aren't indented.

## The `WithAttrs` method

One of `slog`'s performance optimizations is support for pre-formatting
attributes. The `Logger.With` method converts key-value pairs into `Attr`s and
then calls `Handler.WithAttrs`.
The handler may store the attributes for later consumption by the `Handle` method,
or it may take the opportunity to format the attributes now, once,
rather than doing so repeatedly on each call to `Handle`.

The signature of the `WithAttrs` method is

    WithAttrs(attrs []Attr) Handler

The attributes are the processed key-value pairs passed to `Logger.With`.
The return value should be a new instance of your handler that contains
the attributes, possibly pre-formatted.

`WithAttrs` must return a new handler with the additional attributes, leaving
the original handler (its receiver) unchanged. For example, this call:

    logger2 := logger1.With("k", v)

creates a new logger, `logger2`, with an additional attribute, but has no
effect on `logger1`.

We will show example implementations of `WithAttrs` below, when we discuss `WithGroup`.

## The `WithGroup` method

`Logger.WithGroup` calls `Handler.WithGroup` directly, with the same
argument, the group name.
A handler should remember the name so it can use it to qualify all subsequent
attributes.

The signature of `WithGroup` is:

    WithGroup(name string) Handler

Like `WithAttrs`, the `WithGroup` method should return a new handler, not modify
the receiver.

The implementations of `WithGroup` and `WithAttrs` are intertwined.
Consider this statement:

    logger = logger.WithGroup("g1").With("k1", 1).WithGroup("g2").With("k2", 2)

Subsequent `logger` output should qualify key "k1" with group "g1",
and key "k2" with groups "g1" and "g2".
The order of the `Logger.WithGroup` and `Logger.With` calls must be respected by
the implementations of `Handler.WithGroup` and `Handler.WithAttrs`.

We will look at two implementations of `WithGroup` and `WithAttrs`, one that pre-formats and
one that doesn't.

### Without pre-formatting

Our first implementation will collect the information from `WithGroup` and
`WithAttrs` calls to build up a slice of group names and attribute lists,
and loop over that slice in `Handle`. We start with a struct that can hold
either a group name or some attributes:

%include indenthandler2/indent_handler.go gora -

Then we add a slice of `groupOrAttrs` to our handler:

%include indenthandler2/indent_handler.go IndentHandler -

As stated above, The `WithGroup` and `WithAttrs` methods should not modify their
receiver.
To that end, we define a method that will copy our handler struct
and append one `groupOrAttrs` to the copy:

%include indenthandler2/indent_handler.go withgora -

Most of the fields of `IndentHandler` can be copied shallowly, but the slice of
`groupOrAttrs` requires a deep copy, or the clone and the original will point to
the same underlying array. If we used `append` instead of making an explicit
copy, we would introduce that subtle aliasing bug.

The `With` methods are easy to write using `withGroupOrAttrs`:

%include indenthandler2/indent_handler.go withs -

The `Handle` method can now process the groupOrAttrs slice after
the built-in attributes and before the ones in the record:

%include indenthandler2/indent_handler.go handle -

You may have noticed that our algorithm for
recording `WithGroup` and `WithAttrs` information is quadratic in the
number of calls to those methods, because of the repeated copying.
That is unlikely to matter in practice, but if it bothers you,
you can use a linked list instead,
which `Handle` will have to reverse or visit recursively.
See the
[github.com/jba/slog/withsupport](https://github.com/jba/slog/tree/main/withsupport)
package for an implementation.

#### Getting the mutex right

Let us revisit the last few lines of `Handle`:

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(buf)
    return err

This code hasn't changed, but we can now appreciate why `h.mu` is a
pointer to a `sync.Mutex`. Both `WithGroup` and `WithAttrs` copy the handler.
Both copies point to the same mutex.
If the copy and the original used different mutexes and were used concurrently,
then their output could be interleaved, or some output could be lost.
Code like this:

    l2 := l1.With("a", 1)
    go l1.Info("hello")
    l2.Info("goodbye")

could produce output like this:

    hegoollo a=dbye1

See [this bug report](https://go.dev/issue/61321) for more detail.


### With pre-formatting

Our second version of the `WithGroup` and `WithAttrs` methods provides pre-formatting.
This implementation is more complicated than the previous one.
Is the extra complexity worth it?
That depends on your circumstances, but here is one circumstance where
it might be.
Say that you wanted your server to log a lot of information about an incoming
request with every log message that happens during that request. A typical
handler might look something like this:

    func (s *Server) handleWidgets(w http.ResponseWriter, r *http.Request) {
        logger := s.logger.With(
            "url", r.URL,
            "traceID": r.Header.Get("X-Cloud-Trace-Context"),
            // many other attributes
            )
        // ...
    }

A single `handleWidgets` request might generate hundreds of log lines.
For instance, it might contain code like this:

    for _, w := range widgets {
        logger.Info("processing widget", "name", w.Name)
        // ...
    }

For every such line, the `Handle` method we wrote above will format all
the attributes that were added using `With` above, in addition to the
ones on the log line itself.

Maybe all that extra work doesn't slow down your server significantly, because
it does so much other work that time spent logging is just noise.
But perhaps your server is fast enough that all that extra formatting appears high up
in your CPU profiles. That is when pre-formatting can make a big difference,
by formatting the attributes in a call to `With` just once.

To pre-format the arguments to `WithAttrs`, we need to keep track of some
additional state in the `IndentHandler` struct.

%include indenthandler3/indent_handler.go IndentHandler -

Mainly, we need a buffer to hold the pre-formatted data.
But we also need to keep track of which groups
we've seen but haven't output yet. We'll call those groups "unopened."
We also need to track how many groups we've opened, which we can do
with a simple counter, since an opened group's only effect is to change the
indentation level.

This `WithGroup` is a lot like the previous one: it just remembers the
new group, which is unopened initially.

%include indenthandler3/indent_handler.go WithGroup -

`WithAttrs` does all the pre-formatting:

%include indenthandler3/indent_handler.go WithAttrs -

It first opens any unopened groups. This handles calls like:

    logger.WithGroup("g").WithGroup("h").With("a", 1)

Here, `WithAttrs` must output "g" and "h" before "a". Since a group established
by `WithGroup` is in effect for the rest of the log line, `WithAttrs` increments
the indentation level for each group it opens.

Lastly, `WithAttrs` formats its argument attributes, using the same `appendAttr`
method we saw above.

It's the `Handle` method's job to insert the pre-formatted material in the right
place, which is after the built-in attributes and before the ones in the record:

%include indenthandler3/indent_handler.go Handle -

It must also open any groups that haven't yet been opened. The logic covers
log lines like this one:

    logger.WithGroup("g").Info("msg", "a", 1)

where "g" is unopened before `Handle` is called and must be written to produce
the correct output:

    level: INFO
    msg: "msg"
    g:
        a: 1

The check for `r.NumAttrs() > 0` handles this case:

    logger.WithGroup("g").Info("msg")

Here there are no record attributes, so no group to open.

## Testing

The [`Handler` contract](https://pkg.go.dev/log/slog#Handler) specifies several
constraints on handlers.
To verify that your handler follows these rules and generally produces proper
output, use the [testing/slogtest package](https://pkg.go.dev/testing/slogtest).

That package's `TestHandler` function takes an instance of your handler and
a function that returns its output formatted as a slice of maps. Here is the test function
for our example handler:

%include indenthandler3/indent_handler_test.go TestSlogtest -

Calling `TestHandler` is easy. The hard part is parsing your handler's output.
`TestHandler` calls your handler multiple times, resulting in a sequence of log
entries.
It is your job to parse each entry into a `map[string]any`.
A group in an entry should appear as a nested map.

If your handler outputs a standard format, you can use an existing parser.
For example, if your handler outputs one JSON object per line, then you
can split the output into lines and call `encoding/json.Unmarshal` on each.
Parsers for other formats that can unmarshal into a map can be used out
of the box.
Our example output is enough like YAML so that we can use the `gopkg.in/yaml.v3`
package to parse it:

%include indenthandler3/indent_handler_test.go parseLogEntries -

If you have to write your own parser, it can be far from perfect.
The `slogtest` package uses only a handful of simple attributes.
(It is testing handler conformance, not parsing.)
Your parser can ignore edge cases like whitespace and newlines in keys and
values. Before switching to a YAML parser, we wrote an adequate custom parser
in 65 lines.

# General considerations

## Copying records

Most handlers won't need to copy the `slog.Record` that is passed
to the `Handle` method.
Those that do must take special care in some cases.

A handler can make a single copy of a `Record` with an ordinary Go
assignment, channel send or function call if it doesn't retain the
original.
But if its actions result in more than one copy, it should call `Record.Clone`
to make the copies so that they don't share state.
This `Handle` method passes the record to a single handler, so it doesn't require `Clone`:

    type Handler1 struct {
        h slog.Handler
        // ...
    }

    func (h *Handler1) Handle(ctx context.Context, r slog.Record) error {
        return h.h.Handle(ctx, r)
    }

This `Handle` method might pass the record to more than one handler, so it
should use `Clone`:

    type Handler2 struct {
        hs []slog.Handler
        // ...
    }

    func (h *Handler2) Handle(ctx context.Context, r slog.Record) error {
        for _, hh := range h.hs {
            if err := hh.Handle(ctx, r.Clone()); err != nil {
                return err
            }
        }
        return nil
    }

## Concurrency safety

A handler must work properly when a single `Logger` is shared among several
goroutines.
That means that mutable state must be protected with a lock or some other mechanism.
In practice, this is not hard to achieve, because many handlers won't have any
mutable state.

- The `Enabled` method typically consults only its arguments and a configured
  level. The level is often either set once initially, or is held in a
  `LevelVar`, which is already concurrency-safe.

- The `WithAttrs` and `WithGroup` methods should not modify the receiver,
  for reasons discussed above.

- The `Handle` method typically works only with its arguments and stored fields.

Calls to output methods like `io.Writer.Write` should be synchronized unless
it can be verified that no locking is needed.
As we saw in our example, storing a pointer to a mutex enables a logger and
all of its clones to synchronize with each other.
Beware of facile claims like "Unix writes are atomic"; the situation is a lot more nuanced than that.

Some handlers have legitimate reasons for keeping state.
For example, a handler might support a `SetLevel` method to change its configured level
dynamically.
Or it might output the time between successive calls to `Handle`,
which requires a mutable field holding the last output time.
Synchronize all accesses to such fields, both reads and writes.

The built-in handlers have no directly mutable state.
They use a mutex only to sequence calls to their contained `io.Writer`.

## Robustness

Logging is often the debugging technique of last resort. When it is difficult or
impossible to inspect a system, as is typically the case with a production
server, logs provide the most detailed way to understand its behavior.
Therefore, your handler should be robust to bad input.

For example, the usual advice when a function discovers a problem,
like an invalid argument, is to panic or return an error.
The built-in handlers do not follow that advice.
Few things are more frustrating than being unable to debug a problem that
causes logging to fail;
it is better to produce some output, however imperfect, than to produce none at all.
That is why methods like `Logger.Info` convert programming bugs in their list of
key-value pairs, like missing values or malformed keys,
into `Attr`s that contain as much information as possible.

One place to avoid panics is in processing attribute values. A handler that wants
to format a value will probably switch on the value's kind:

    switch attr.Value.Kind() {
    case KindString: ...
    case KindTime: ...
    // all other Kinds
    default: ...
    }

What should happen in the default case, when the handler encounters a `Kind`
that it doesn't know about?
The built-in handlers try to muddle through by using the result of the value's
`String` method, as our example handler does.
They do not panic or return an error.
Your own handlers might in addition want to report the problem through your production monitoring
or error-tracking telemetry system.
The most likely explanation for the issue is that a newer version of the `slog` package added
a new `Kind`&mdash;a backwards-compatible change under the Go 1 Compatibility
Promise&mdash;and the handler wasn't updated.
That is certainly a problem, but it shouldn't deprive
readers from seeing the rest of the log output.

There is one circumstance where returning an error from `Handler.Handle` is appropriate.
If the output operation itself fails, the best course of action is to report
this failure by returning the error. For instance, the last two lines of the
built-in `Handle` methods are

    _, err := h.w.Write(*state.buf)
    return err

Although the output methods of `Logger` ignore the error, one could write a
handler that does something with it, perhaps falling back to writing to standard
error.

## Speed

Most programs don't need fast logging.
Before making your handler fast, gather data&mdash;preferably production data,
not benchmark comparisons&mdash;that demonstrates that it needs to be fast.
Avoid premature optimization.

If you need a fast handler, start with pre-formatting. It may provide dramatic
speed-ups in cases where a single call to `Logger.With` is followed by many
calls to the resulting logger.

If log output is the bottleneck, consider making your handler asynchronous.
Do the minimal amount of processing in the handler, then send the record and
other information over a channel. Another goroutine can collect the incoming log
entries and write them in bulk and in the background.
You might want to preserve the option to log synchronously
so you can see all the log output to debug a crash.

Allocation is often a major cause of a slow system.
The `slog` package already works hard at minimizing allocations.
If your handler does its own allocation, and profiling shows it to be
a problem, then see if you can minimize it.

One simple change you can make is to replace calls to `fmt.Sprintf` or `fmt.Appendf`
with direct appends to the buffer. For example, our IndentHandler appends string
attributes to the buffer like so:

	buf = fmt.Appendf(buf, "%s: %q\n", a.Key, a.Value.String())

As of Go 1.21, that results in two allocations, one for each argument passed to
an `any` parameter. We can get that down to zero by using `append` directly:

	buf = append(buf, a.Key...)
	buf = append(buf, ": "...)
	buf = strconv.AppendQuote(buf, a.Value.String())
	buf = append(buf, '\n')

Another worthwhile change is to use a `sync.Pool` to manage the one chunk of
memory that most handlers need:
the `[]byte` buffer holding the formatted output.

Our example `Handle` method began with this line:

	buf := make([]byte, 0, 1024)

As we said above, providing a large initial capacity avoids repeated copying and
re-allocation of the slice as it grows, reducing the number of allocations to
one.
But we can get it down to zero in the steady state by keeping a global pool of buffers.
Initially, the pool will be empty and new buffers will be allocated.
But eventually, assuming the number of concurrent log calls reaches a steady
maximum, there will be enough buffers in the pool to share among all the
ongoing `Handler` calls. As long as no log entry grows past a buffer's capacity,
there will be no allocations from the garbage collector's point of view.

We will hide our pool behind a pair of functions, `allocBuf` and `freeBuf`.
The single line to get a buffer at the top of `Handle` becomes two lines:

	bufp := allocBuf()
	defer freeBuf(bufp)

One of the subtleties involved in making a `sync.Pool` of slices
is suggested by the variable name `bufp`: your pool must deal in
_pointers_ to slices, not the slices themselves.
Pooled values must always be pointers. If they aren't, then the `any` arguments
and return values of the `sync.Pool` methods will themselves cause allocations,
defeating the purpose of pooling.

There are two ways to proceed with our slice pointer: we can replace `buf`
with `*bufp` throughout our function, or we can dereference it and remember to
re-assign it before freeing:

	bufp := allocBuf()
	buf := *bufp
	defer func() {
		*bufp = buf
		freeBuf(bufp)
	}()


Here is our pool and its associated functions:

%include indenthandler4/indent_handler.go pool -

The pool's `New` function does the same thing as the original code:
create a byte slice with 0 length and plenty of capacity.
The `allocBuf` function just type-asserts the result of the pool's
`Get` method.

The `freeBuf` method truncates the buffer before putting it back
in the pool, so that `allocBuf` always returns zero-length slices.
It also implements an important optimization: it doesn't return
large buffers to the pool.
To see why this important, consider what would happen if there were a single,
unusually large log entry&mdash;say one that was a megabyte when formatted.
If that megabyte-sized buffer were put in the pool, it could remain
there indefinitely, constantly being reused, but with most of its capacity
wasted.
The extra memory might never be used again by the handler, and since it was in
the handler's pool, it might never be garbage-collected for reuse elsewhere.
We can avoid that situation by excluding large buffers from the pool.