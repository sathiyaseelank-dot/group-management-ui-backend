/**
 * Mock API for Zero-Trust Identity Provider
 * Simulates backend endpoints with ~100ms delay
 */

import {
  User,
  Group,
  ServiceAccount,
  GroupMember,
  Resource,
  AccessRule,
  Subject,
  SelectedSubject,
} from './types';

// Mock Data: Users
const MOCK_USERS: User[] = [
  {
    id: 'usr_1',
    name: 'Alice Johnson',
    type: 'USER',
    displayLabel: 'User: Alice Johnson',
    email: 'alice@company.com',
    status: 'active',
    groups: ['grp_1', 'grp_3'],
    createdAt: '2026-01-10',
  },
  {
    id: 'usr_2',
    name: 'Bob Smith',
    type: 'USER',
    displayLabel: 'User: Bob Smith',
    email: 'bob@company.com',
    status: 'active',
    groups: ['grp_2'],
    createdAt: '2026-01-12',
  },
  {
    id: 'usr_3',
    name: 'Charlie Davis',
    type: 'USER',
    displayLabel: 'User: Charlie Davis',
    email: 'charlie@company.com',
    status: 'active',
    groups: ['grp_1'],
    createdAt: '2026-01-15',
  },
  {
    id: 'usr_4',
    name: 'Diana Wilson',
    type: 'USER',
    displayLabel: 'User: Diana Wilson',
    email: 'diana@company.com',
    status: 'inactive',
    groups: [],
    createdAt: '2026-02-01',
  },
];

// Mock Data: Groups
const MOCK_GROUPS: Group[] = [
  {
    id: 'grp_1',
    name: 'Engineering',
    type: 'GROUP',
    displayLabel: 'Group: Engineering',
    description: 'Engineering team with database and API access',
    memberCount: 2,
    createdAt: '2026-01-15',
  },
  {
    id: 'grp_2',
    name: 'Marketing',
    type: 'GROUP',
    displayLabel: 'Group: Marketing',
    description: 'Marketing department',
    memberCount: 1,
    createdAt: '2026-01-20',
  },
  {
    id: 'grp_3',
    name: 'Admin',
    type: 'GROUP',
    displayLabel: 'Group: Admin',
    description: 'System administrators',
    memberCount: 1,
    createdAt: '2026-01-25',
  },
];

// Mock Data: Service Accounts
const MOCK_SERVICE_ACCOUNTS: ServiceAccount[] = [
  {
    id: 'svc_1',
    name: 'CI/CD Pipeline',
    type: 'SERVICE',
    displayLabel: 'Service: CI/CD Pipeline',
    status: 'active',
    associatedResourceCount: 2,
    createdAt: '2026-01-01',
  },
  {
    id: 'svc_2',
    name: 'Analytics Sync',
    type: 'SERVICE',
    displayLabel: 'Service: Analytics Sync',
    status: 'active',
    associatedResourceCount: 1,
    createdAt: '2026-01-10',
  },
];

// Mock Data: Resources
const MOCK_RESOURCES: Resource[] = [
  {
    id: 'res_1',
    name: 'Database Server',
    address: 'db.internal.company.com:5432',
    description: 'Production PostgreSQL database for main application',
  },
  {
    id: 'res_2',
    name: 'API Gateway',
    address: 'api.company.com',
    description: 'Main API endpoint for frontend applications',
  },
  {
    id: 'res_3',
    name: 'S3 Bucket',
    address: 'company-assets.s3.amazonaws.com',
    description: 'Asset storage bucket',
  },
];

// Mock Data: Access Rules
const MOCK_ACCESS_RULES: AccessRule[] = [
  {
    id: 'rule_1',
    resourceId: 'res_1',
    subjectId: 'grp_1',
    subjectType: 'GROUP',
    subjectName: 'Engineering',
    effect: 'ALLOW',
    createdAt: '2026-01-20',
  },
  {
    id: 'rule_2',
    resourceId: 'res_2',
    subjectId: 'grp_1',
    subjectType: 'GROUP',
    subjectName: 'Engineering',
    effect: 'ALLOW',
    createdAt: '2026-01-20',
  },
  {
    id: 'rule_3',
    resourceId: 'res_3',
    subjectId: 'svc_1',
    subjectType: 'SERVICE',
    subjectName: 'CI/CD Pipeline',
    effect: 'ALLOW',
    createdAt: '2026-01-21',
  },
  {
    id: 'rule_4',
    resourceId: 'res_2',
    subjectId: 'svc_2',
    subjectType: 'SERVICE',
    subjectName: 'Analytics Sync',
    effect: 'ALLOW',
    createdAt: '2026-01-22',
  },
];

// Mock Data: Group Membership
const MOCK_GROUP_MEMBERS: Record<string, GroupMember[]> = {
  grp_1: [
    { userId: 'usr_1', userName: 'Alice Johnson', email: 'alice@company.com' },
    { userId: 'usr_3', userName: 'Charlie Davis', email: 'charlie@company.com' },
  ],
  grp_2: [
    { userId: 'usr_2', userName: 'Bob Smith', email: 'bob@company.com' },
  ],
  grp_3: [
    { userId: 'usr_1', userName: 'Alice Johnson', email: 'alice@company.com' },
  ],
};

// Utility: Simulate API delay
const delay = (ms: number = 100) =>
  new Promise((resolve) => setTimeout(resolve, ms));

// API: Get all subjects (Users, Groups, Service Accounts)
export async function getSubjects(): Promise<Subject[]> {
  await delay();
  const subjects: Subject[] = [
    ...MOCK_USERS,
    ...MOCK_GROUPS,
    ...MOCK_SERVICE_ACCOUNTS,
  ];
  return subjects;
}

// API: Get subjects filtered by type
export async function getSubjectsByType(
  type?: 'USER' | 'GROUP' | 'SERVICE'
): Promise<Subject[]> {
  const subjects = await getSubjects();
  if (!type) return subjects;
  return subjects.filter((s) => s.type === type);
}

// API: Get all groups
export async function getGroups(): Promise<Group[]> {
  await delay();
  return MOCK_GROUPS;
}

// API: Get single group with members
export async function getGroup(groupId: string) {
  await delay();
  const group = MOCK_GROUPS.find((g) => g.id === groupId);
  const members = MOCK_GROUP_MEMBERS[groupId] || [];
  
  // Get resources this group has access to
  const groupResources = MOCK_ACCESS_RULES
    .filter((rule) => rule.subjectId === groupId && rule.subjectType === 'GROUP')
    .map((rule) => MOCK_RESOURCES.find((r) => r.id === rule.resourceId))
    .filter(Boolean) as Resource[];

  return { group, members, resources: groupResources };
}

// API: Get all users
export async function getUsers(): Promise<User[]> {
  await delay();
  return MOCK_USERS;
}

// API: Get all service accounts
export async function getServiceAccounts(): Promise<ServiceAccount[]> {
  await delay();
  return MOCK_SERVICE_ACCOUNTS;
}

// API: Get single resource with access rules
export async function getResource(resourceId: string) {
  await delay();
  const resource = MOCK_RESOURCES.find((r) => r.id === resourceId);
  const accessRules = MOCK_ACCESS_RULES.filter((r) => r.resourceId === resourceId);
  return { resource, accessRules };
}

// API: Get all resources
export async function getResources(): Promise<Resource[]> {
  await delay();
  return MOCK_RESOURCES;
}

// API: Update group membership
export async function updateGroupMembers(
  groupId: string,
  memberIds: string[]
): Promise<void> {
  await delay(150);
  // In real implementation, this would POST to backend
  const members = MOCK_USERS.filter((u) => memberIds.includes(u.id));
  MOCK_GROUP_MEMBERS[groupId] = members.map((u) => ({
    userId: u.id,
    userName: u.name,
    email: u.email,
  }));
}

// API: Create access rule
export async function createAccessRule(
  resourceId: string,
  subjects: SelectedSubject[],
  effect: 'ALLOW' | 'DENY'
): Promise<void> {
  await delay(150);
  // In real implementation, this would POST to backend
  subjects.forEach((subject) => {
    const subjectName =
      MOCK_USERS.find((u) => u.id === subject.id)?.name ||
      MOCK_GROUPS.find((g) => g.id === subject.id)?.name ||
      MOCK_SERVICE_ACCOUNTS.find((s) => s.id === subject.id)?.name ||
      subject.label;

    MOCK_ACCESS_RULES.push({
      id: `rule_${Date.now()}_${subject.id}`,
      resourceId,
      subjectId: subject.id,
      subjectType: subject.type,
      subjectName,
      effect,
      createdAt: new Date().toISOString().split('T')[0],
    });
  });
}

// API: Delete access rule
export async function deleteAccessRule(ruleId: string): Promise<void> {
  await delay(150);
  const index = MOCK_ACCESS_RULES.findIndex((r) => r.id === ruleId);
  if (index !== -1) {
    MOCK_ACCESS_RULES.splice(index, 1);
  }
}

// API: Delete group member
export async function removeGroupMember(
  groupId: string,
  userId: string
): Promise<void> {
  await delay(150);
  if (MOCK_GROUP_MEMBERS[groupId]) {
    MOCK_GROUP_MEMBERS[groupId] = MOCK_GROUP_MEMBERS[groupId].filter(
      (m) => m.userId !== userId
    );
  }
}
