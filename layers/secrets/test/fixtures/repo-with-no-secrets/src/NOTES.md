# Notes on credentials

Documentation about secrets is not a secret. An AWS access key id begins with AKIA
and is twenty characters long; a private key file begins with a BEGIN header. Writing
that down has to stay possible, or the first thing anybody does with a scanner is turn
it off.

Tokens live in the environment. See `settings.py`.
