// A test that mentions jwtVerify proves nothing about the Worker that answers.
import { jwtVerify } from "jose";
export const unused = jwtVerify;
