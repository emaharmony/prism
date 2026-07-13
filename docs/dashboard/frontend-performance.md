# Frontend performance

The embedded dashboard avoids a frontend framework and large dependencies. The
overview makes bounded concurrent requests to existing endpoints, renders only the
latest task/activity subset, uses one shared request helper with cancellation and a
timeout, and does not start unbounded polling. Data refresh is user initiated.
