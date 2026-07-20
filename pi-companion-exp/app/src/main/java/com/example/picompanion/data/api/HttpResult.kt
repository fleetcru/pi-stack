package com.example.picompanion.data.api

sealed interface HttpResult<out T> {
  data class Success<T>(val value: T) : HttpResult<T>
  data class Failure(val message: String, val code: Int = -1, val cause: Throwable? = null) : HttpResult<Nothing> {
    val isNetworkError: Boolean get() = code == -1
    val isAuthError: Boolean get() = code == 401 || code == 403
    val isServerError: Boolean get() = code in 500..599
    val isNotFound: Boolean get() = code == 404
    val userMessage: String get() = when {
      isAuthError -> "Authentication failed — check your token"
      code == 404 -> "Resource not found"
      isServerError -> "Server error ($code)"
      isNetworkError -> message // "Connection failed: ..."
      else -> message
    }
  }
}

inline fun <T, R> HttpResult<T>.map(transform: (T) -> R): HttpResult<R> = when (this) {
  is HttpResult.Success -> HttpResult.Success(transform(value))
  is HttpResult.Failure -> this
}

inline fun <T> HttpResult<T>.onSuccess(action: (T) -> Unit): HttpResult<T> {
  if (this is HttpResult.Success) action(value)
  return this
}

inline fun <T> HttpResult<T>.onFailure(action: (HttpResult.Failure) -> Unit): HttpResult<T> {
  if (this is HttpResult.Failure) action(this)
  return this
}

fun <T> HttpResult<T>.getOrNull(): T? = when (this) {
  is HttpResult.Success -> value
  is HttpResult.Failure -> null
}
