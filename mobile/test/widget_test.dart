import 'package:flutter_test/flutter_test.dart';

import 'package:eino_rag_mobile/theme/app_theme.dart';

void main() {
  test('AppTheme exposes accent color', () {
    expect(AppTheme.accent.value, isNonZero);
  });
}
