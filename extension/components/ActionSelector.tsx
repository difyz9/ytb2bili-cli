import React, { useState } from 'react';
import { ActionType } from '../types';

interface ActionSelectorProps {
  onActionSelect: (action: ActionType) => void;
}

export const ActionSelector: React.FC<ActionSelectorProps> = ({ onActionSelect }) => {
  const [isOpen, setIsOpen] = useState(false);

  const handleAction = (action: ActionType) => {
    onActionSelect(action);
    setIsOpen(false);
  };

  return (
    <div className="fixed bottom-8 right-8 z-[9999]">
      {isOpen && (
        <div className="mb-4 bg-white rounded-lg shadow-xl p-2 min-w-[200px]">
          <button
            onClick={() => handleAction(ActionType.SAVE_URL)}
            className="w-full px-4 py-2 text-left hover:bg-gray-100 rounded transition-colors flex items-center gap-2"
          >
            <svg className="w-5 h-5 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
            </svg>
            <span className="text-gray-800">保存当前 URL</span>
          </button>
          <button
            onClick={() => handleAction(ActionType.SAVE_SUBTITLE)}
            className="w-full px-4 py-2 text-left hover:bg-gray-100 rounded transition-colors flex items-center gap-2"
          >
            <svg className="w-5 h-5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
            </svg>
            <span className="text-gray-800">保存字幕</span>
          </button>
        </div>
      )}
      
      <button
        onClick={() => setIsOpen(!isOpen)}
        className={`w-14 h-14 rounded-full shadow-lg flex items-center justify-center transition-all ${
          isOpen 
            ? 'bg-red-500 hover:bg-red-600 rotate-45' 
            : 'bg-blue-500 hover:bg-blue-600'
        }`}
      >
        <svg className="w-6 h-6 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
        </svg>
      </button>
    </div>
  );
};
