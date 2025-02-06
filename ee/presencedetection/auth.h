//go:build darwin
// +build darwin

// auth.h
#ifndef AUTH_H
#define AUTH_H

#include <stdint.h>
#include <stdbool.h>

struct AuthResult {
    bool success;       // true for success, false for failure
    char* error_msg;    // Error message if any
    int error_code;     // Error code if any
};

// Function to authenticate with a timeout (in nanoseconds)
struct AuthResult Authenticate(const char *reason, int64_t timeout_ns);

#endif
