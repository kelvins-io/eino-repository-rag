class ApiException implements Exception {
  final String message;
  final int? statusCode;
  final bool forceLogout;

  ApiException(
    this.message, {
    this.statusCode,
    this.forceLogout = false,
  });

  @override
  String toString() => message;
}
