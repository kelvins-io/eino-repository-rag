import 'package:flutter/material.dart';

import '../api/models.dart';
import '../theme/app_theme.dart';

/// 扁平化目录树，便于下拉选择。
List<({int id, String label, int depth})> flattenDirectories(
  List<DirectoryNode> nodes, {
  int depth = 0,
}) {
  final out = <({int id, String label, int depth})>[];
  for (final n in nodes) {
    final prefix = depth == 0 ? '' : '${'··' * depth} ';
    out.add((id: n.id, label: '$prefix${n.name}', depth: depth));
    out.addAll(flattenDirectories(n.children, depth: depth + 1));
  }
  return out;
}

class KnowledgeBaseSelector extends StatelessWidget {
  const KnowledgeBaseSelector({
    super.key,
    required this.items,
    required this.value,
    required this.onChanged,
    this.enabled = true,
  });

  final List<KnowledgeBase> items;
  final int? value;
  final ValueChanged<int?> onChanged;
  final bool enabled;

  @override
  Widget build(BuildContext context) {
    return DropdownButtonFormField<int>(
      value: value != null && items.any((e) => e.id == value) ? value : null,
      isExpanded: true,
      decoration: const InputDecoration(
        labelText: '知识库',
      ),
      items: [
        for (final kb in items)
          DropdownMenuItem(
            value: kb.id,
            child: Text(kb.name, overflow: TextOverflow.ellipsis),
          ),
      ],
      onChanged: enabled ? onChanged : null,
    );
  }
}

class DirectorySelector extends StatelessWidget {
  const DirectorySelector({
    super.key,
    required this.tree,
    required this.value,
    required this.onChanged,
    this.enabled = true,
    this.allowClear = true,
  });

  final List<DirectoryNode> tree;
  final int? value;
  final ValueChanged<int?> onChanged;
  final bool enabled;
  final bool allowClear;

  @override
  Widget build(BuildContext context) {
    final flat = flattenDirectories(tree);
    final valid =
        value != null && flat.any((e) => e.id == value) ? value : null;
    return DropdownButtonFormField<int?>(
      value: valid,
      isExpanded: true,
      decoration: const InputDecoration(
        labelText: '目录（可选）',
      ),
      items: [
        if (allowClear)
          const DropdownMenuItem<int?>(
            value: null,
            child: Text('全部目录', style: TextStyle(color: AppTheme.textSecondary)),
          ),
        for (final d in flat)
          DropdownMenuItem<int?>(
            value: d.id,
            child: Text(d.label, overflow: TextOverflow.ellipsis),
          ),
      ],
      onChanged: enabled ? onChanged : null,
    );
  }
}
