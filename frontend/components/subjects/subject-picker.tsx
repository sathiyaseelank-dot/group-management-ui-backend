'use client';

import { useEffect, useState, useCallback } from 'react';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { getSubjects } from '@/lib/mock-api';
import { Subject, SelectedSubject, SubjectType } from '@/lib/types';
import { X, Loader2 } from 'lucide-react';

interface SubjectPickerProps {
  selectedSubjects?: SelectedSubject[];
  onSelectionChange?: (subjects: SelectedSubject[]) => void;
  subjectTypes?: SubjectType[];
  excludeSubjects?: string[]; // Subject IDs to exclude from selection
}

/**
 * Core Security Primitive: Reusable Subject Picker
 *
 * Purpose: Select subjects (Users, Groups, Service Accounts) for ACL policies
 * Used by: Add Members modal, Add Access Rule modal
 *
 * Features:
 * - Single API call loads all subjects (cached for instant picker interactions)
 * - Tabs filter client-side (Users, Groups, Services)
 * - Searchable by name
 * - Multi-select with checkboxes
 * - Selected list display with remove action
 */
export function SubjectPicker({
  selectedSubjects = [],
  onSelectionChange,
  subjectTypes = ['USER', 'GROUP', 'SERVICE'],
  excludeSubjects = [],
}: SubjectPickerProps) {
  const [allSubjects, setAllSubjects] = useState<Subject[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [activeTab, setActiveTab] = useState<SubjectType>(
    subjectTypes.includes('USER') ? 'USER' : subjectTypes[0]
  );

  // Load all subjects once on mount
  useEffect(() => {
    const loadSubjects = async () => {
      try {
        const subjects = await getSubjects();
        setAllSubjects(subjects);
      } catch (error) {
        console.error('Failed to load subjects:', error);
      } finally {
        setLoading(false);
      }
    };

    loadSubjects();
  }, []);

  // Get subjects for current tab, filtered by search
  const filteredSubjects = allSubjects.filter((subject) => {
    const matchesType = subject.type === activeTab;
    const name = subject.name ?? '';
    const matchesSearch = name.toLowerCase().includes(searchQuery.toLowerCase());
    const notExcluded = !excludeSubjects.includes(subject.id);
    return matchesType && matchesSearch && notExcluded;
  });

  // Handle subject selection toggle
  const toggleSubject = useCallback(
    (subject: Subject) => {
      const isSelected = selectedSubjects.some((s) => s.id === subject.id);
      const newSelection = isSelected
        ? selectedSubjects.filter((s) => s.id !== subject.id)
        : [
            ...selectedSubjects,
            {
              id: subject.id,
              type: subject.type,
              label: subject.displayLabel,
            },
          ];
      onSelectionChange?.(newSelection);
    },
    [selectedSubjects, onSelectionChange]
  );

  // Remove a selected subject
  const removeSelected = useCallback(
    (id: string) => {
      const newSelection = selectedSubjects.filter((s) => s.id !== id);
      onSelectionChange?.(newSelection);
    },
    [selectedSubjects, onSelectionChange]
  );

  // Check if a subject is selected
  const isSelected = (id: string) =>
    selectedSubjects.some((s) => s.id === id);

  // Build available tabs based on subjectTypes prop
  const availableTabs = subjectTypes.filter((type) =>
    allSubjects.some((s) => s.type === type)
  );

  return (
    <div className="flex flex-col gap-4">
      {/* Tabs for filtering by subject type */}
      <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as SubjectType)}>
        <TabsList className="grid w-full grid-cols-3">
          {availableTabs.includes('USER') && <TabsTrigger value="USER">Users</TabsTrigger>}
          {availableTabs.includes('GROUP') && <TabsTrigger value="GROUP">Groups</TabsTrigger>}
          {availableTabs.includes('SERVICE') && <TabsTrigger value="SERVICE">Services</TabsTrigger>}
        </TabsList>

        {/* Search Input */}
        <div className="mt-3">
          <Input
            placeholder={`Search ${activeTab.toLowerCase()}s...`}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="mb-3"
            disabled={loading}
          />
        </div>

        {/* Subject List by Tab */}
        {availableTabs.includes('USER') && (
          <TabsContent value="USER" className="max-h-64 overflow-y-auto">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : filteredSubjects.length === 0 ? (
              <div className="py-8 text-center text-sm text-muted-foreground">
                {searchQuery ? 'No users found' : 'No users available'}
              </div>
            ) : (
              <div className="space-y-2">
                {filteredSubjects.map((subject) => (
                  <div
                    key={subject.id}
                    className="flex items-center gap-3 rounded-lg border p-3 hover:bg-accent"
                  >
                    <Checkbox
                      checked={isSelected(subject.id)}
                      onCheckedChange={() => toggleSubject(subject)}
                      id={subject.id}
                    />
                    <label
                      htmlFor={subject.id}
                      className="flex flex-1 cursor-pointer items-center gap-2"
                    >
                      <span className="text-sm font-medium">{subject.name}</span>
                      <Badge variant="outline" className="text-xs">
                        {subject.type}
                      </Badge>
                    </label>
                  </div>
                ))}
              </div>
            )}
          </TabsContent>
        )}

        {availableTabs.includes('GROUP') && (
          <TabsContent value="GROUP" className="max-h-64 overflow-y-auto">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : filteredSubjects.length === 0 ? (
              <div className="py-8 text-center text-sm text-muted-foreground">
                {searchQuery ? 'No groups found' : 'No groups available'}
              </div>
            ) : (
              <div className="space-y-2">
                {filteredSubjects.map((subject) => (
                  <div
                    key={subject.id}
                    className="flex items-center gap-3 rounded-lg border p-3 hover:bg-accent"
                  >
                    <Checkbox
                      checked={isSelected(subject.id)}
                      onCheckedChange={() => toggleSubject(subject)}
                      id={subject.id}
                    />
                    <label
                      htmlFor={subject.id}
                      className="flex flex-1 cursor-pointer items-center gap-2"
                    >
                      <span className="text-sm font-medium">{subject.name}</span>
                      <Badge variant="outline" className="text-xs">
                        {subject.type}
                      </Badge>
                    </label>
                  </div>
                ))}
              </div>
            )}
          </TabsContent>
        )}

        {availableTabs.includes('SERVICE') && (
          <TabsContent value="SERVICE" className="max-h-64 overflow-y-auto">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : filteredSubjects.length === 0 ? (
              <div className="py-8 text-center text-sm text-muted-foreground">
                {searchQuery ? 'No services found' : 'No services available'}
              </div>
            ) : (
              <div className="space-y-2">
                {filteredSubjects.map((subject) => (
                  <div
                    key={subject.id}
                    className="flex items-center gap-3 rounded-lg border p-3 hover:bg-accent"
                  >
                    <Checkbox
                      checked={isSelected(subject.id)}
                      onCheckedChange={() => toggleSubject(subject)}
                      id={subject.id}
                    />
                    <label
                      htmlFor={subject.id}
                      className="flex flex-1 cursor-pointer items-center gap-2"
                    >
                      <span className="text-sm font-medium">{subject.name}</span>
                      <Badge variant="outline" className="text-xs">
                        {subject.type}
                      </Badge>
                    </label>
                  </div>
                ))}
              </div>
            )}
          </TabsContent>
        )}
      </Tabs>

      {/* Selected Subjects Display */}
      {selectedSubjects.length > 0 && (
        <div className="space-y-2 border-t pt-3">
          <p className="text-xs font-semibold text-muted-foreground">
            Selected ({selectedSubjects.length})
          </p>
          <div className="flex flex-wrap gap-2">
            {selectedSubjects.map((selected) => (
              <Badge key={selected.id} variant="secondary" className="gap-2">
                {selected.label}
                <button
                  onClick={() => removeSelected(selected.id)}
                  className="hover:text-foreground/80"
                >
                  <X className="h-3 w-3" />
                </button>
              </Badge>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
