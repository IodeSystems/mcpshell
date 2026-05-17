# event_emitter

**Status:** PASS
**Duration:** 3808ms
**Tool calls:** 2

## Prompt

> In mcpshell, implement a simple event emitter with on(event, handler) and emit(event, data) methods. Register two handlers for 'data' event: one that returns data as-is, one that returns data * 2. Emit with value 42, collect all handler results into an array.

## Final Answer

```
[42, 84]
```

## Attempts

### Attempt 1 (ERROR)

```javascript
// Simple event emitter implementation
let EventEmitter = {
  handlers: {},
  
  on: function(event, handler) {
    if (!this.handlers[event]) {
      this.handlers[event] = [];
    }
    this.handlers[event].push(handler);
  },
  
  emit: function(event, data) {
    if (!this.handlers[event]) {
      return [];
    }
    return this.handlers[event].map(handler => handler(data));
  }
};

// Create instance
let emitter = EventEmitter;

// Register two handlers for 'data' event
emitter.on('data', (data) => data);
emitter.on('data', (data) => data * 2);

// Emit with value 42 and collect results
let results = emitter.emit('data', 42);

results
```

**Result:**
```
ERROR: mcpshell does not support 'this' — there are no classes or methods

  at line 24:10
```

### Attempt 2 (OK)

```javascript
// Simple event emitter implementation without 'this'
let handlers = {};

let on = function(event, handler) {
  if (!handlers[event]) {
    handlers[event] = [];
  }
  handlers[event].push(handler);
};

let emit = function(event, data) {
  if (!handlers[event]) {
    return [];
  }
  return handlers[event].map(handler => handler(data));
};

// Register two handlers for 'data' event
on('data', (data) => data);
on('data', (data) => data * 2);

// Emit with value 42 and collect results
let results = emit('data', 42);

results
```

**Result:**
```
[42, 84]
```

