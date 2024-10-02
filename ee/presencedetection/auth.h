//go:build darwin
// +build darwin

// auth.h
#ifndef AUTH_H
#define AUTH_H

#include <stdbool.h>

struct AuthResult {
    bool success;       // true for success, false for failure
    char* error_msg;    // Error message if any
    int error_code;     // Error code if any
};

struct AuthResult Authenticate(char const* reason);

#endif
