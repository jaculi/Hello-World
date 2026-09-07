// 站点树组件（M19）：把扁平 Site 列表组织成 parent→children 树并渲染。
import Link from "next/link";
import type { Site } from "@/lib/api";

export interface SiteTreeProps {
  sites: Site[];
}

interface TreeNode extends Site {
  children: TreeNode[];
}

function buildTree(sites: Site[]): TreeNode[] {
  const byId = new Map<string, TreeNode>();
  sites.forEach((s) => byId.set(s.site_id, { ...s, children: [] }));
  const roots: TreeNode[] = [];
  byId.forEach((node) => {
    if (node.parent_site_id && byId.has(node.parent_site_id)) {
      byId.get(node.parent_site_id)!.children.push(node);
    } else {
      roots.push(node);
    }
  });
  return roots;
}

function NodeView({ node, depth }: { node: TreeNode; depth: number }) {
  return (
    <li>
      <div
        className="flex items-center gap-2 py-1"
        style={{ paddingLeft: `${depth * 1.25}rem` }}
      >
        <Link
          href={`/sites/${node.site_id}`}
          className="text-sm text-brand-600 hover:underline"
        >
          {node.display_name}
        </Link>
        <span className="text-xs text-slate-400">
          {node.site_type} · <code className="text-slate-400">{node.name}</code>
        </span>
      </div>
      {node.children.length > 0 && (
        <ul className="border-l border-slate-200">
          {node.children
            .sort((a, b) => a.name.localeCompare(b.name))
            .map((c) => (
              <NodeView key={c.site_id} node={c} depth={depth + 1} />
            ))}
        </ul>
      )}
    </li>
  );
}

export function SiteTree({ sites }: SiteTreeProps) {
  const roots = buildTree(sites);
  if (roots.length === 0) {
    return <p className="text-sm text-slate-400">该租户暂无站点</p>;
  }
  return (
    <ul className="space-y-0.5">
      {roots.map((r) => (
        <NodeView key={r.site_id} node={r} depth={0} />
      ))}
    </ul>
  );
}
